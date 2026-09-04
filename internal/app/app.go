package app

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cao7113/gater/internal/app/types"
	"github.com/cao7113/gater/internal/config"
)

// - 本地macOS应用代理工具，运行要求：绝对正确&安全，并发和性能要求不高
// - 要求结构清晰简单，注释完整

// State 表示应用当前所处的运行状态
type State string

const (
	StateStopped  State = "stopped"
	StateStarting State = "starting"
	StateRunning  State = "running"
	StateCrashed  State = "crashed"
)

type App struct {
	mu          sync.RWMutex
	Config      config.AppConfig
	Port        int
	State       State
	LastActive  time.Time
	Timeout     time.Duration
	Cmd         *exec.Cmd
	processDone chan error
	Pid         int
	RuntimeEnv  map[string]string
	StartedAt   *time.Time

	Proxy         *httputil.ReverseProxy
	LogBuf        *LogBuffer
	StartupMs     int64
	LastStartedAt *time.Time
}

// NewApp 创建并初始化应用实例
func NewApp(ac config.AppConfig) *App {
	timeout := 10 * time.Minute
	if ac.IdleTimeout != "" {
		if d, err := time.ParseDuration(ac.IdleTimeout); err == nil {
			timeout = d
		}
	}

	// 1. 生产级 Transport 配置（优化连接池与超时，避免连接泄露）
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   2 * time.Second, // 快速感知连接建立失败
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	application := &App{
		Config:     ac,
		Port:       DynamicPort,
		State:      StateStopped,
		Timeout:    timeout,
		LastActive: time.Now(),
		LogBuf:     NewLogBuffer(1000),
		RuntimeEnv: make(map[string]string),
	}

	// 2. 使用 Rewrite 并配置优雅的 ErrorHandler（防止 502/断连导致进程崩溃）
	application.Proxy = &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			application.mu.RLock()
			port := application.Port
			application.mu.RUnlock()
			r.SetURL(&url.URL{Scheme: "http", Host: net.JoinHostPort(config.TargetHost, strconv.Itoa(port))})
			// 透传真实请求 Header
			r.Out.Header.Set("X-Forwarded-Host", r.In.Host)
		},
		Transport:    transport,
		ErrorHandler: NewProxyErrorHandler(ac.Name), // 独立函数调用,
	}

	return application
}

// ============================================================================
// 1. 流程控制与生命周期管理 (Flow Control & Lifecycle)
// ============================================================================

// Run 保证应用处于运行状态。使用大粒度锁保证绝对的线程安全与顺序化启动
func (a *App) Run(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.LastActive = time.Now()

	// 1. 状态检查：如果已经在运行，直接成功返回
	if a.State == StateRunning {
		return nil
	}

	// 1.1 如果当前处于启动中，提示返回，避免排队后重复触发启动
	if a.State == StateStarting {
		return fmt.Errorf("应用 [%s] 正在启动中，请稍后...", a.Config.Name)
	}

	// 2. 标记进入启动中状态，便于外部监控感知
	a.State = StateStarting
	startTime := time.Now()

	// 3. 执行启动流程
	if err := a.startAppLocked(ctx); err != nil {
		a.State = StateCrashed
		_, _ = a.LogBuf.Write([]byte(err.Error() + "\n"))
		a.cleanCmdLocked()
		a.releasePortLocked()
		return err
	}

	// 4. 启动成功，更新状态为 Running 并记录元数据
	a.State = StateRunning
	a.StartupMs = time.Since(startTime).Milliseconds()
	now := time.Now()
	a.LastStartedAt = &now
	a.StartedAt = &now
	if a.Cmd != nil && a.Cmd.Process != nil {
		a.Pid = a.Cmd.Process.Pid
	}
	log.Printf("[Gater] [%s] 应用就绪，耗时 %dms，运行于 %s", a.Config.Name, a.StartupMs, net.JoinHostPort(config.TargetHost, strconv.Itoa(a.Port)))

	return nil
}

// startAppLocked 具体的启动核心逻辑（必须在持有 a.mu 的情况下调用）
func (a *App) startAppLocked(ctx context.Context) error {
	// 1. 检查运行环境与命令路径
	// 空 cwd 表示继承 Gater 的当前工作目录；填写 cwd 时才校验目录。
	if a.Config.Cwd != "" {
		if fi, err := os.Stat(a.Config.Cwd); err != nil || !fi.IsDir() {
			return fmt.Errorf("应用工作目录不存在: %s", a.Config.Cwd)
		}
	}
	if _, err := exec.LookPath(a.Config.Cmd); err != nil {
		return fmt.Errorf("未找到启动命令 [%s]，请确认是否已安装或检查 PATH 环境变量", a.Config.Cmd)
	}
	if a.Config.Port > 0 {
		a.Port = a.Config.Port
	} else {
		port, err := NextPort()
		if err != nil {
			return err
		}
		a.Port = port
	}

	// 2. 构建类型上下文并执行启动前 Hook
	appTypeContext := a.newAppTypeContext()
	appTypeHandler := types.HandlerFor(a.Config.AppType)
	if err := appTypeHandler.Prepare(appTypeContext); err != nil {
		return fmt.Errorf("准备应用运行环境失败: %w", err)
	}
	if err := appTypeHandler.BeforeStart(ctx, appTypeContext); err != nil {
		return fmt.Errorf("应用启动前检查失败: %w", err)
	}

	// 3. 执行子进程创建
	cmd := exec.Command(a.Config.Cmd, appTypeContext.Args...)
	// Setpgid: 进程组隔离
	// Pdeathsig: 主进程意外退出时，系统自动发送 SIGKILL 终止子进程，杜绝孤儿进程
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		// 存在跨平台问题 todo
		// Pdeathsig: syscall.SIGKILL,
	}
	cmd.Dir = a.Config.Cwd
	cmd.Env = ToEnvList(appTypeContext.Env) // 复用本地 utils.go 中的 ToEnvList

	output := io.MultiWriter(os.Stdout, a.LogBuf)
	cmd.Stdout = output
	cmd.Stderr = output

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("进程启动失败: %w", err)
	}

	processDone := make(chan error, 1)
	a.Cmd = cmd
	a.processDone = processDone

	// 启动后台 Goroutine 等待子进程退出
	go a.waitTargetProcess(cmd, processDone)

	// 4. 等待端口打开（同时监听进程是否闪退退出）
	if err := a.waitForPortOrExit(ctx, processDone); err != nil {
		a.stopCmdLocked(syscall.SIGKILL)
		return err
	}

	// 5. 执行启动后 Hook
	if err := appTypeHandler.AfterStart(ctx, appTypeContext); err != nil {
		a.stopCmdLocked(syscall.SIGKILL)
		return fmt.Errorf("应用启动后处理失败: %w", err)
	}

	a.RuntimeEnv = cloneEnvMap(appTypeContext.Env)
	if a.Cmd != nil && a.Cmd.Process != nil {
		a.Pid = a.Cmd.Process.Pid
	}
	return nil
}

// Stop 停止应用并清理关联进程
func (a *App) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.State == StateStopped {
		return
	}

	log.Printf("[Gater] [%s] 正在停止应用...", a.Config.Name)
	a.stopCmdLocked(syscall.SIGTERM) // 优先尝试 SIGTERM 优雅退出，超时后自动强杀
	a.State = StateStopped
	a.Pid = 0
	a.releasePortLocked()
	log.Printf("[Gater] [%s] 进程组已彻底清理", a.Config.Name)
}

// MonitorIdle 定时巡检空闲超时，超时后自动停止应用
func (a *App) MonitorIdle(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			a.mu.RLock()
			isIdle := a.State == StateRunning && time.Since(a.LastActive) > a.Timeout
			a.mu.RUnlock()

			if isIdle {
				a.Stop()
			}
		case <-ctx.Done():
			return
		}
	}
}

// Touch 刷新应用的最后活跃时间
func (a *App) Touch() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.LastActive = time.Now()
}

// GetState 供外部在不持有大锁长时间等待的情况下获取实时状态
func (a *App) GetState() State {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.State
}

// releasePortLocked 清理当前运行实例的端口，调用方必须持有 a.mu。
func (a *App) releasePortLocked() {
	a.Port = DynamicPort
}

// ============================================================================
// 2. 进程与底层辅助逻辑 (Process & Helper Routines)
// ============================================================================

// waitTargetProcess 在后台等待子进程退出，处理意外崩溃
func (a *App) waitTargetProcess(cmd *exec.Cmd, processDone chan<- error) {
	err := cmd.Wait()

	// 非阻塞发送：避免端口检查超时退出后导致的 Goroutine 永久死锁泄露
	select {
	case processDone <- err:
	default:
	}
	// 在获取 a.mu 前关闭通道，唤醒等待 stopCmdLocked 的主线程，防止死锁
	close(processDone)

	a.mu.Lock()
	defer a.mu.Unlock()

	// 仅在当前 Cmd 未被替换且处于运行状态时，认定为意外退出
	if a.Cmd == cmd && a.State == StateRunning {
		log.Printf("[Gater] [%s] 进程意外退出: %v", a.Config.Name, err)
		a.State = StateCrashed
		a.Pid = 0
		a.releasePortLocked()
	}
}

// 轮询检查 TCP 端口，同时监视进程是否闪退
func (a *App) waitForPortOrExit(ctx context.Context, processDone <-chan error) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	addr := net.JoinHostPort(config.TargetHost, strconv.Itoa(a.Port))
	dialTimeout := 300 * time.Millisecond

	for {
		select {
		case <-timeoutCtx.Done():
			return fmt.Errorf("等待端口 %s 监听超时", addr)

		case err := <-processDone:
			return fmt.Errorf("应用进程在启动就绪前已闪退退出: %v", err)

		case <-ticker.C:
			conn, err := net.DialTimeout("tcp", addr, dialTimeout)
			if err == nil {
				conn.Close()
				return nil
			}
		}
	}
}

// stopCmdLocked 向进程组发送信号并等待退出（必须在持有 a.mu 的情况下调用）
func (a *App) stopCmdLocked(signal syscall.Signal) {
	cmd := a.Cmd
	processDone := a.processDone
	a.cleanCmdLocked()

	if cmd == nil || cmd.Process == nil {
		return
	}

	if signal == 0 {
		signal = syscall.SIGTERM
	}

	// 1. 获取进程组 ID (PGID) 并发送信号 kill 整个进程组
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err == nil {
		_ = syscall.Kill(-pgid, signal)
	} else {
		_ = cmd.Process.Signal(signal)
	}

	// 2. 优雅退场机制：等待退出，5 秒超时后触发强制强杀 (SIGKILL)
	if processDone != nil {
		select {
		case <-processDone:
			// 进程已顺利退出
		case <-time.After(5 * time.Second):
			log.Printf("[Gater] [%s] 进程响应信号超时，执行强制强杀 (SIGKILL)", a.Config.Name)
			if pgid > 0 {
				_ = syscall.Kill(-pgid, syscall.SIGKILL)
			} else {
				_ = cmd.Process.Kill()
			}
		}
	}
}

// cleanCmdLocked 重置进程指针字段
func (a *App) cleanCmdLocked() {
	a.Cmd = nil
	a.processDone = nil
}

// ============================================================================
// 3. 环境变量与域名解析工具 (Env & Domain Helpers)
// ============================================================================

func (a *App) newAppTypeContext() *types.TypeContext {
	domain := a.Domain()

	// 仅用于占位符替换的基础变量
	ctxVars := map[string]string{
		"APP_NAME":   a.Config.Name,
		"APP_DOMAIN": domain,
		"PORT":       strconv.Itoa(a.Port),
	}

	// 读取系统环境变量
	sysEnviron := os.Environ()
	envMap := make(map[string]string, len(sysEnviron)+len(a.Config.Env)+len(ctxVars))
	for _, e := range sysEnviron {
		if kv := strings.SplitN(e, "=", 2); len(kv) == 2 {
			envMap[kv[0]] = kv[1]
		}
	}

	// 写入基础变量
	// for k, v := range ctxVars {
	// 	envMap[k] = v
	// }

	// 复用本地 utils.go 中的 ExpandPlaceholders 展开 Config.Env 并写入 envMap
	for k, v := range a.Config.Env {
		envMap[k] = ExpandPlaceholders(v, ctxVars)
	}

	// 复用本地 utils.go 中的 ExpandSlice 展开 Config.Args
	args := ExpandSlice(a.Config.Args, ctxVars)

	return &types.TypeContext{
		Config:     a.Config,
		Domain:     domain,
		Port:       a.Port,
		WorkingDir: a.Config.Cwd,
		Args:       args,
		Env:        envMap,
	}
}

func (a *App) Domain() string {
	name := strings.TrimSpace(a.Config.Name)
	suffix := strings.TrimSpace(a.Config.DomainSuffix)
	if name == "" || suffix == "" {
		return ""
	}
	return name + suffix
}

func (a *App) URL(schemes ...string) string {
	host := a.Domain()
	if host == "" {
		return ""
	}
	scheme := config.ParseAppSuffix(a.Config.DomainSuffix).Scheme
	if len(schemes) > 0 && strings.TrimSpace(schemes[0]) != "" {
		scheme = strings.TrimSpace(schemes[0])
	}
	return (&url.URL{Scheme: scheme, Host: host}).String()
}

func (a *App) Snapshot() map[string]any {
	return a.snapshot(false)
}

func (a *App) SnapshotWithSensitive(showSensitive bool) map[string]any {
	return a.snapshot(showSensitive)
}

func (a *App) snapshot(showSensitive bool) map[string]any {
	a.mu.RLock()
	defer a.mu.RUnlock()

	env := cloneEnvMap(a.RuntimeEnv)
	if a.State != StateRunning {
		env = cloneEnvMap(a.Config.Env)
	}
	if env == nil {
		env = map[string]string{}
	}
	if !showSensitive {
		env = sanitizeEnvMap(env)
	}
	payload := map[string]any{
		"name":         a.Config.Name,
		"state":        string(a.State),
		"cwd":          a.Config.Cwd,
		"cmd":          a.Config.Cmd,
		"args":         append([]string(nil), a.Config.Args...),
		"env":          env,
		"config_port":  a.Config.Port,
		"port":         a.Port,
		"pid":          a.Pid,
		"startup_ms":   a.StartupMs,
		"started_at":   a.StartedAt,
		"last_started": a.LastStartedAt,
		"idle_timeout": a.Timeout.String(),
	}
	return payload
}
