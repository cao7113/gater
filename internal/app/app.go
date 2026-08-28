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
	"strings"
	"sync"
	"syscall"
	"time"

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

func (a *App) EnsureStarted(ctx context.Context, domain string) error {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return fmt.Errorf("缺少 APP_DOMAIN")
	}
	a.mu.Lock()
	a.LastActive = time.Now()

	if a.State == StateRunning {
		a.mu.Unlock()
		return nil
	}
	if a.State == StateStarting {
		a.mu.Unlock()
		return a.waitReady(ctx)
	}

	a.State = StateStarting
	startTime := time.Now()
	a.mu.Unlock()

	// 1. 校验应用工作目录是否存在
	if fi, err := os.Stat(a.Config.Cwd); err != nil || !fi.IsDir() {
		a.mu.Lock()
		a.State = StateCrashed
		errMsg := fmt.Sprintf("应用工作目录不存在: %s", a.Config.Cwd)
		_, _ = a.LogBuf.Write([]byte(errMsg))
		a.mu.Unlock()
		return fmt.Errorf("%s", errMsg)
	}

	// 2. 校验启动命令是否在系统 PATH 中或有效
	if _, err := exec.LookPath(a.Config.Cmd); err != nil {
		a.mu.Lock()
		a.State = StateCrashed
		errMsg := fmt.Sprintf("未找到启动命令 [%s]，请确认是否已安装或检查 PATH 环境变量", a.Config.Cmd)
		_, _ = a.LogBuf.Write([]byte(errMsg))
		a.mu.Unlock()
		return fmt.Errorf("%s", errMsg)
	}

	var finalArgs []string
	for _, arg := range a.Config.Args {
		finalArgs = append(finalArgs, os.Expand(arg, func(k string) string {
			if k == "PORT" {
				return fmt.Sprintf("%d", a.Port)
			}
			return os.Getenv(k)
		}))
	}

	cmd := exec.Command(a.Config.Cmd, finalArgs...)
	cmd.Dir = a.Config.Cwd

	// 创建独立进程组，防孤儿进程
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	multiWriter := io.MultiWriter(os.Stdout, a.LogBuf)
	cmd.Stdout = multiWriter
	cmd.Stderr = multiWriter

	cmd.Env = a.environment(domain)

	if err := cmd.Start(); err != nil {
		a.mu.Lock()
		a.State = StateCrashed
		_, _ = a.LogBuf.Write(fmt.Appendf(nil, "进程启动失败: %v", err))
		a.mu.Unlock()
		return fmt.Errorf("进程启动失败: %w", err)
	}

	a.mu.Lock()
	a.Cmd = cmd
	a.mu.Unlock()

	// 监听进程非正常退出
	go func() {
		err := cmd.Wait()
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.State == StateRunning {
			log.Printf("[Gater] [%s] 进程意外退出: %v", a.Config.Name, err)
			a.State = StateCrashed
		}
	}()

	err := a.waitReady(ctx)

	a.mu.Lock()
	defer a.mu.Unlock()
	if err != nil {
		if a.Cmd != nil && a.Cmd.Process != nil {
			_ = syscall.Kill(-a.Cmd.Process.Pid, syscall.SIGKILL)
		}
		a.State = StateCrashed
		return err
	}

	a.State = StateRunning
	a.StartupMs = time.Since(startTime).Milliseconds()
	startedAt := time.Now()
	a.LastStartedAt = &startedAt

	log.Printf("[Gater] [%s] 应用就绪，耗时 %dms，运行于 127.0.0.1:%d", a.Config.Name, a.StartupMs, a.Port)
	return nil
}

func (a *App) environment(domain string) []string {
	envMap := make(map[string]string)
	for _, e := range os.Environ() {
		kv := strings.SplitN(e, "=", 2)
		if len(kv) == 2 {
			envMap[kv[0]] = kv[1]
		}
	}
	for k, v := range a.Config.Env {
		envMap[k] = v
	}
	envMap["PORT"] = fmt.Sprintf("%d", a.Port)
	envMap["APP_DOMAIN"] = domain
	if strings.EqualFold(strings.TrimSpace(a.Config.AppType), "phx") {
		envMap["PHX_HOST"] = domain
	}

	var envList []string
	for k, v := range envMap {
		envList = append(envList, fmt.Sprintf("%s=%s", k, v))
	}
	return envList
}

func (a *App) waitReady(ctx context.Context) error {
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

func (a *App) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.Cmd == nil || a.Cmd.Process == nil {
		a.State = StateStopped
		return
	}

	pid := a.Cmd.Process.Pid
	log.Printf("[Gater] [%s] 正在终止进程组 (PGID: %d)...", a.Config.Name, pid)

	// 向整个进程组发送 SIGINT 信号
	_ = syscall.Kill(-pid, syscall.SIGINT)

	done := make(chan struct{})
	go func() {
		_ = a.Cmd.Wait()
		close(done)
	}()

	select {
	case <-time.After(3 * time.Second):
		log.Printf("[Gater] [%s] 终止超时，强行杀死进程组", a.Config.Name)
		_ = syscall.Kill(-pid, syscall.SIGKILL)
	case <-done:
	}

	a.State = StateStopped
	a.Cmd = nil
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

func (a *App) Touch() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.LastActive = time.Now()
}
