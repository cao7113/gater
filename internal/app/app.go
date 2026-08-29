package app

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
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

// State 表示应用当前所处的运行状态
type State string

const (
	StateStopped  State = "stopped"
	StateStarting State = "starting"
	StateRunning  State = "running"
	StateCrashed  State = "crashed"
)

type App struct {
	mu          sync.Mutex
	Config      config.AppConfig
	Port        int
	State       State
	LastActive  time.Time
	Timeout     time.Duration
	Cmd         *exec.Cmd
	processDone chan error

	Proxy         *httputil.ReverseProxy
	LogBuf        *LogBuffer
	StartupMs     int64
	LastStartedAt *time.Time
}

// NewApp 创建并初始化应用实例
func NewApp(ac config.AppConfig, port int) *App {
	timeout := 10 * time.Minute
	if ac.IdleTimeout != "" {
		if d, err := time.ParseDuration(ac.IdleTimeout); err == nil {
			timeout = d
		}
	}

	targetURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))

	// 使用 Go 1.18+ 推荐的 Rewrite 替代被废弃的 NewSingleHostReverseProxy
	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(targetURL)
		},
	}

	return &App{
		Config:     ac,
		Port:       port,
		State:      StateStopped,
		Timeout:    timeout,
		LastActive: time.Now(),
		Proxy:      proxy,
		LogBuf:     NewLogBuffer(1000),
	}
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

	// 2. 标记进入启动中状态，便于外部监控感知
	a.State = StateStarting
	startTime := time.Now()

	// 3. 执行启动流程
	if err := a.startAppLocked(ctx); err != nil {
		a.State = StateCrashed
		_, _ = a.LogBuf.Write([]byte(err.Error()))
		a.cleanCmdLocked()
		return err
	}

	// 4. 启动成功，更新状态为 Running 并记录元数据
	a.State = StateRunning
	a.StartupMs = time.Since(startTime).Milliseconds()
	now := time.Now()
	a.LastStartedAt = &now
	log.Printf("[Gater] [%s] 应用就绪，耗时 %dms，运行于 127.0.0.1:%d", a.Config.Name, a.StartupMs, a.Port)

	return nil
}

// startAppLocked 具体的启动核心逻辑（必须在持有 a.mu 的情况下调用）
func (a *App) startAppLocked(ctx context.Context) error {
	// 1. 检查运行环境与命令路径
	if fi, err := os.Stat(a.Config.Cwd); err != nil || !fi.IsDir() {
		return fmt.Errorf("应用工作目录不存在: %s", a.Config.Cwd)
	}
	if _, err := exec.LookPath(a.Config.Cmd); err != nil {
		return fmt.Errorf("未找到启动命令 [%s]，请确认是否已安装或检查 PATH 环境变量", a.Config.Cmd)
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
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Dir = a.Config.Cwd
	cmd.Env = envList(appTypeContext.Env)

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
	go a.waitProcess(cmd, processDone)

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
	a.stopCmdLocked(syscall.SIGINT)
	a.State = StateStopped
	log.Printf("[Gater] [%s] 进程组已彻底清理", a.Config.Name)
}

// MonitorIdle 定时巡检空闲超时，超时后自动停止应用
func (a *App) MonitorIdle(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			a.mu.Lock()
			isIdle := a.State == StateRunning && time.Since(a.LastActive) > a.Timeout
			a.mu.Unlock()

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
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.State
}

// ============================================================================
// 2. 进程与底层辅助逻辑 (Process & Helper Routines)
// ============================================================================

// waitProcess 在后台等待子进程退出，处理意外崩溃
func (a *App) waitProcess(cmd *exec.Cmd, processDone chan<- error) {
	err := cmd.Wait()
	processDone <- err
	close(processDone)

	a.mu.Lock()
	defer a.mu.Unlock()

	// 仅在当前 Cmd 未被替换且处于运行状态时，认定为意外退出
	if a.Cmd == cmd && a.State == StateRunning {
		log.Printf("[Gater] [%s] 进程意外退出: %v", a.Config.Name, err)
		a.State = StateCrashed
	}
}

// waitForPortOrExit 轮询检查 TCP 端口，同时监视进程是否闪退
func (a *App) waitForPortOrExit(ctx context.Context, processDone <-chan error) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	addr := fmt.Sprintf("127.0.0.1:%d", a.Port)
	for {
		select {
		case <-timeoutCtx.Done():
			return fmt.Errorf("等待端口 %s 监听超时", addr)

		case err := <-processDone:
			return fmt.Errorf("应用进程在启动就绪前已闪退退出: %v", err)

		case <-ticker.C:
			conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
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

	// 杀掉进程组
	_ = syscall.Kill(-cmd.Process.Pid, signal)

	if processDone != nil {
		select {
		case <-processDone:
		case <-time.After(3 * time.Second):
			// 超时未退出则强制杀进程
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
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
		"DOMAIN_HOST": domain,
		"PORT":        strconv.Itoa(a.Port),
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
	for k, v := range ctxVars {
		envMap[k] = v
	}

	// 使用 ctxVars 展开 Config.Env 并写入 envMap
	for k, v := range a.Config.Env {
		envMap[k] = ExpandPlaceholders(v, ctxVars)
	}

	// 展开 Config.Args
	args := ExpandSlice(a.Config.Args, ctxVars)

	return &types.TypeContext{
		Config: a.Config,
		// AppType:    a.Config.AppType, // 👈【修复点】显式补充 AppType 字段
		Domain:     domain,
		Port:       a.Port,
		WorkingDir: a.Config.Cwd,
		Args:       args,
		Env:        envMap,
	}
}

// ExpandPlaceholders 使用 vars 字典展开字符串中的 ${VAR} 或 $VAR 占位符
func ExpandPlaceholders(s string, vars map[string]string) string {
	return os.Expand(s, func(k string) string {
		return vars[k]
	})
}

// ExpandSlice 批量展开切片中的占位符
func ExpandSlice(slice []string, vars map[string]string) []string {
	result := make([]string, len(slice))
	for i, v := range slice {
		result[i] = ExpandPlaceholders(v, vars)
	}
	return result
}

func envList(envMap map[string]string) []string {
	envList := make([]string, 0, len(envMap))
	for k, v := range envMap {
		envList = append(envList, fmt.Sprintf("%s=%s", k, v))
	}
	return envList
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
