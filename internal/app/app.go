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

	"github.com/cao7113/gater/internal/store"
)

type State string

const (
	StateStopped  State = "stopped"
	StateStarting State = "starting"
	StateRunning  State = "running"
	StateCrashed  State = "crashed"
)

type LogBuffer struct {
	mu       sync.RWMutex
	lines    []string
	maxLines int
}

func NewLogBuffer(maxLines int) *LogBuffer {
	return &LogBuffer{
		lines:    make([]string, 0, maxLines),
		maxLines: maxLines,
	}
}

func (b *LogBuffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	s := string(p)
	rawLines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for i, line := range rawLines {
		if i == len(rawLines)-1 && line == "" {
			continue
		}
		if len(b.lines) >= b.maxLines {
			b.lines = b.lines[1:]
		}
		b.lines = append(b.lines, fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), line))
	}
	return len(p), nil
}

func (b *LogBuffer) String() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return strings.Join(b.lines, "\n")
}

func (b *LogBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lines = b.lines[:0]
}

type App struct {
	mu         sync.Mutex
	Spec       store.AppSpec
	Port       int
	State      State
	LastActive time.Time
	Timeout    time.Duration
	Cmd        *exec.Cmd
	Proxy      *httputil.ReverseProxy
	LogBuf     *LogBuffer
}

func NewApp(spec store.AppSpec, port int) *App {
	timeout := 5 * time.Minute
	if spec.IdleTimeout != "" {
		if d, err := time.ParseDuration(spec.IdleTimeout); err == nil {
			timeout = d
		}
	}

	targetURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	return &App{
		Spec:       spec,
		Port:       port,
		State:      StateStopped,
		Timeout:    timeout,
		LastActive: time.Now(),
		Proxy:      httputil.NewSingleHostReverseProxy(targetURL),
		LogBuf:     NewLogBuffer(1000),
	}
}

func (a *App) EnsureStarted(ctx context.Context) error {
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
	a.mu.Unlock()

	// 1. 校验应用工作目录是否存在
	if fi, err := os.Stat(a.Spec.Path); err != nil || !fi.IsDir() {
		a.mu.Lock()
		a.State = StateCrashed
		errMsg := fmt.Sprintf("应用工作目录不存在: %s", a.Spec.Path)
		_, _ = a.LogBuf.Write([]byte(errMsg))
		a.mu.Unlock()
		return fmt.Errorf("%s", errMsg)
	}

	// 2. 校验启动命令是否在系统 PATH 中或有效
	if _, err := exec.LookPath(a.Spec.Cmd); err != nil {
		a.mu.Lock()
		a.State = StateCrashed
		errMsg := fmt.Sprintf("未找到启动命令 [%s]，请确认是否已安装或检查 PATH 环境变量", a.Spec.Cmd)
		_, _ = a.LogBuf.Write([]byte(errMsg))
		a.mu.Unlock()
		return fmt.Errorf("%s", errMsg)
	}

	var finalArgs []string
	for _, arg := range a.Spec.Args {
		finalArgs = append(finalArgs, os.Expand(arg, func(k string) string {
			if k == "PORT" {
				return fmt.Sprintf("%d", a.Port)
			}
			return os.Getenv(k)
		}))
	}

	cmd := exec.Command(a.Spec.Cmd, finalArgs...)
	cmd.Dir = a.Spec.Path

	// 创建独立进程组，防孤儿进程
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	multiWriter := io.MultiWriter(os.Stdout, a.LogBuf)
	cmd.Stdout = multiWriter
	cmd.Stderr = multiWriter

	envMap := make(map[string]string)
	for _, e := range os.Environ() {
		kv := strings.SplitN(e, "=", 2)
		if len(kv) == 2 {
			envMap[kv[0]] = kv[1]
		}
	}
	for k, v := range a.Spec.Env {
		envMap[k] = v
	}
	envMap["PORT"] = fmt.Sprintf("%d", a.Port)

	var envList []string
	for k, v := range envMap {
		envList = append(envList, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = envList

	if err := cmd.Start(); err != nil {
		a.mu.Lock()
		a.State = StateCrashed
		_, _ = a.LogBuf.Write([]byte(fmt.Sprintf("进程启动失败: %v", err)))
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
			log.Printf("[Gater] [%s] 进程意外退出: %v", a.Spec.Name, err)
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
	log.Printf("[Gater] [%s] 应用就绪，运行于 127.0.0.1:%d", a.Spec.Name, a.Port)
	return nil
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
	log.Printf("[Gater] [%s] 正在终止进程组 (PGID: %d)...", a.Spec.Name, pid)

	// 向整个进程组发送 SIGINT 信号
	_ = syscall.Kill(-pid, syscall.SIGINT)

	done := make(chan struct{})
	go func() {
		_ = a.Cmd.Wait()
		close(done)
	}()

	select {
	case <-time.After(3 * time.Second):
		log.Printf("[Gater] [%s] 终止超时，强行杀死进程组", a.Spec.Name)
		_ = syscall.Kill(-pid, syscall.SIGKILL)
	case <-done:
	}

	a.State = StateStopped
	a.Cmd = nil
	log.Printf("[Gater] [%s] 进程组已彻底清理", a.Spec.Name)
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
	a.LastActive = time.Now()
	a.mu.Unlock()
}
