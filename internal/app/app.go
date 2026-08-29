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

type State string

const (
	StateStopped  State = "stopped"
	StateStarting State = "starting"
	StateRunning  State = "running"
	StateCrashed  State = "crashed"
)

type App struct {
	mu            sync.Mutex
	Config        config.AppConfig
	Port          int
	State         State
	LastActive    time.Time
	Timeout       time.Duration
	Cmd           *exec.Cmd
	processDone   chan error
	Proxy         *httputil.ReverseProxy
	LogBuf        *LogBuffer
	StartupMs     int64
	LastStartedAt *time.Time
}

func NewApp(ac config.AppConfig, port int) *App {
	timeout := 10 * time.Minute
	if ac.IdleTimeout != "" {
		if d, err := time.ParseDuration(ac.IdleTimeout); err == nil {
			timeout = d
		}
	}

	// todo config
	targetURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	return &App{
		Config:     ac,
		Port:       port,
		State:      StateStopped,
		Timeout:    timeout,
		LastActive: time.Now(),
		Proxy:      httputil.NewSingleHostReverseProxy(targetURL),
		LogBuf:     NewLogBuffer(1000),
	}
}

func (a *App) EnsureRunning(ctx context.Context) error {
	startTime, shouldWait := a.beginStart()
	if shouldWait {
		return a.waitForPort(ctx)
	}
	if startTime.IsZero() {
		return nil
	}

	appTypeContext, appTypeHandler, err := a.prepareStart(ctx)
	if err != nil {
		return a.failStart(err)
	}

	if err := a.startProcess(appTypeContext); err != nil {
		return a.failStart(err)
	}

	if err := a.finalizeStart(ctx, appTypeHandler, appTypeContext, startTime); err != nil {
		return a.failStart(err)
	}
	return nil
}

func (a *App) beginStart() (time.Time, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.LastActive = time.Now()
	switch a.State {
	case StateRunning:
		return time.Time{}, false
	case StateStarting:
		return time.Time{}, true
	default:
		a.State = StateStarting
		return time.Now(), false
	}
}

func (a *App) prepareStart(ctx context.Context) (*types.TypeContext, types.Handler, error) {
	if fi, err := os.Stat(a.Config.Cwd); err != nil || !fi.IsDir() {
		return nil, nil, fmt.Errorf("应用工作目录不存在: %s", a.Config.Cwd)
	}
	if _, err := exec.LookPath(a.Config.Cmd); err != nil {
		return nil, nil, fmt.Errorf("未找到启动命令 [%s]，请确认是否已安装或检查 PATH 环境变量", a.Config.Cmd)
	}

	appTypeContext := a.newAppTypeContext()
	appTypeHandler := types.HandlerFor(a.Config.AppType)
	if err := appTypeHandler.Prepare(appTypeContext); err != nil {
		return nil, nil, fmt.Errorf("准备应用运行环境失败: %w", err)
	}
	if err := appTypeHandler.BeforeStart(ctx, appTypeContext); err != nil {
		return nil, nil, fmt.Errorf("应用启动前检查失败: %w", err)
	}
	return appTypeContext, appTypeHandler, nil
}

func (a *App) startProcess(appTypeContext *types.TypeContext) error {
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
	a.mu.Lock()
	a.Cmd = cmd
	a.processDone = processDone
	a.mu.Unlock()
	go a.waitProcess(cmd, processDone)
	return nil
}

func (a *App) waitProcess(cmd *exec.Cmd, processDone chan<- error) {
	err := cmd.Wait()
	processDone <- err

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.Cmd == cmd && a.State == StateRunning {
		log.Printf("[Gater] [%s] 进程意外退出: %v", a.Config.Name, err)
		a.State = StateCrashed
	}
}

func (a *App) finalizeStart(ctx context.Context, handler types.Handler, appTypeContext *types.TypeContext, startTime time.Time) error {
	if err := a.waitForPort(ctx); err != nil {
		return err
	}
	if err := handler.AfterStart(ctx, appTypeContext); err != nil {
		return fmt.Errorf("应用启动后处理失败: %w", err)
	}
	a.markRunning(startTime)
	return nil
}

func (a *App) waitForPort(ctx context.Context) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	addr := fmt.Sprintf("127.0.0.1:%d", a.Port)
	for {
		select {
		case <-timeoutCtx.Done():
			return fmt.Errorf("等待端口 %s 监听超时", addr)
		case <-ticker.C:
			conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
			if err == nil {
				conn.Close()
				return nil
			}
		}
	}
}

func (a *App) markRunning(startTime time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.State = StateRunning
	a.StartupMs = time.Since(startTime).Milliseconds()
	startedAt := time.Now()
	a.LastStartedAt = &startedAt
	log.Printf("[Gater] [%s] 应用就绪，耗时 %dms，运行于 127.0.0.1:%d", a.Config.Name, a.StartupMs, a.Port)
}

func (a *App) failStart(err error) error {
	a.stopProcess(syscall.SIGKILL)
	a.markCrashed(err)
	return err
}

func (a *App) stopProcess(signal syscall.Signal) {
	a.mu.Lock()
	cmd := a.Cmd
	processDone := a.processDone
	a.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return
	}

	_ = syscall.Kill(-cmd.Process.Pid, signal)
	if processDone != nil {
		select {
		case <-processDone:
		case <-time.After(3 * time.Second):
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.Cmd == cmd {
		a.Cmd = nil
		a.processDone = nil
	}
}

func (a *App) markCrashed(err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.State = StateCrashed
	_, _ = a.LogBuf.Write([]byte(err.Error()))
}

func (a *App) newAppTypeContext() *types.TypeContext {
	domain := a.Domain()

	// 1. 仅用于占位符替换的上下文字典（不后追加，不混入系统变量）
	ctxVars := map[string]string{
		"DOMAIN_HOST": domain,
		"PORT":        strconv.Itoa(a.Port),
	}

	// 2. 初始化 envMap 并读取系统环境变量
	sysEnviron := os.Environ()
	envMap := make(map[string]string, len(sysEnviron)+len(a.Config.Env)+len(ctxVars))
	for _, e := range sysEnviron {
		if kv := strings.SplitN(e, "=", 2); len(kv) == 2 {
			envMap[kv[0]] = kv[1]
		}
	}

	// 3. 写入固定基础变量
	for k, v := range ctxVars {
		envMap[k] = v
	}

	// 4. 使用 ctxVars 展开 Config.Env 并写入 envMap
	for k, v := range a.Config.Env {
		envMap[k] = ExpandPlaceholders(v, ctxVars)
	}

	// 5. 使用 ctxVars 展开 Config.Args
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

// ExpandPlaceholders 使用给定的 vars 字典展开字符串中的 ${VAR} 或 $VAR 占位符。
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
	var envList []string
	for k, v := range envMap {
		envList = append(envList, fmt.Sprintf("%s=%s", k, v))
	}
	return envList
}

func (a *App) Touch() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.LastActive = time.Now()
}

func (a *App) Stop() {
	a.mu.Lock()
	cmd := a.Cmd
	a.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		a.mu.Lock()
		a.State = StateStopped
		a.mu.Unlock()
		return
	}

	pid := cmd.Process.Pid
	log.Printf("[Gater] [%s] 正在终止进程组 (PGID: %d)...", a.Config.Name, pid)
	a.stopProcess(syscall.SIGINT)

	a.mu.Lock()
	a.State = StateStopped
	a.mu.Unlock()
	log.Printf("[Gater] [%s] 进程组已彻底清理", a.Config.Name)
}

func (a *App) MonitorIdle(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			a.mu.Lock()
			if a.State == StateRunning && time.Since(a.LastActive) > a.Timeout {
				a.mu.Unlock()
				a.Stop()
				continue
			}
			a.mu.Unlock()
		case <-ctx.Done():
			return
		}
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
