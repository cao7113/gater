package app

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cao7113/gater/internal/app/types"
	"github.com/cao7113/gater/internal/config"
)

func TestEnvironmentInjectsPortAndAppDomain(t *testing.T) {
	application := NewApp(config.AppConfig{
		Name:         "demo",
		DomainSuffix: ".l.s",
		Env: map[string]string{
			"FOO": "bar", "aPort": "${PORT}"},
	}, 50001)

	values := make(map[string]string)
	if application.Domain() != "demo.l.s" {
		t.Fatalf("Domain() = %q, want demo.l.s", application.Domain())
	}
	appTypeContext := application.newAppTypeContext()
	if err := types.HandlerFor(application.Config.AppType).Prepare(appTypeContext); err != nil {
		t.Fatal(err)
	}
	for _, item := range envList(appTypeContext.Env) {
		parts := strings.SplitN(item, "=", 2)
		values[parts[0]] = parts[1]
	}

	if values["PORT"] != "50001" {
		t.Fatalf("PORT = %q, want 50001", values["PORT"])
	}
	if values["aPort"] != "50001" {
		t.Fatalf("aPort = %q, want 50001", values["aPort"])
	}
	if values["DOMAIN_HOST"] != "demo.l.s" {
		t.Fatalf("DOMAIN_HOST = %q, want demo.l.s", values["DOMAIN_HOST"])
	}
	if values["FOO"] != "bar" {
		t.Fatalf("FOO = %q, want bar", values["FOO"])
	}
}

func TestURLUsesAppDomain(t *testing.T) {
	application := NewApp(config.AppConfig{Name: "demo", DomainSuffix: ".l.h"}, 50001)
	if got := application.URL(); got != "http://demo.l.h" {
		t.Fatalf("URL() = %q, want http://demo.l.h", got)
	}
	if got := application.URL("https"); got != "https://demo.l.h" {
		t.Fatalf("URL(https) = %q, want https://demo.l.h", got)
	}
}

func TestEnsureRunningRejectsMissingWorkingDirectory(t *testing.T) {
	application := NewApp(config.AppConfig{
		Name:         "demo",
		DomainSuffix: ".l.h",
		Cwd:          "/path/that/does/not/exist",
		Cmd:          "echo",
	}, 50001)

	err := application.Run(context.Background())
	if err == nil {
		t.Fatal("missing working directory was accepted")
	}
	if application.GetState() != StateCrashed {
		t.Fatalf("state = %q, want %q", application.GetState(), StateCrashed)
	}
}

func TestEnsureRunningRejectsMissingCommand(t *testing.T) {
	application := NewApp(config.AppConfig{
		Name:         "demo",
		DomainSuffix: ".l.h",
		Cwd:          t.TempDir(),
		Cmd:          "command-that-does-not-exist",
	}, 50001)

	err := application.Run(context.Background())
	if err == nil {
		t.Fatal("missing command was accepted")
	}
	if application.GetState() != StateCrashed {
		t.Fatalf("state = %q, want %q", application.GetState(), StateCrashed)
	}
}

func TestEnsureRunningRunsAndStopsApplication(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required for the process flow test")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	application := NewApp(config.AppConfig{
		Name:         "demo",
		DomainSuffix: ".l.h",
		Cwd:          t.TempDir(),
		Cmd:          "python3",
		Args:         []string{"-m", "http.server", "$PORT"},
		IdleTimeout:  "1m",
	}, port)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := application.Run(ctx); err != nil {
		t.Fatalf("EnsureRunning() error = %v", err)
	}
	if application.GetState() != StateRunning {
		t.Fatalf("state = %q, want %q", application.GetState(), StateRunning)
	}

	// 验证反向代理请求重写功能 (针对 Go 1.18+ Rewrite 接口)
	req := httptest.NewRequest("GET", "http://demo.l.h/", nil)
	rec := httptest.NewRecorder()
	application.Proxy.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Proxy Response Status = %d, want %d", rec.Code, http.StatusOK)
	}

	application.Stop()
	if application.GetState() != StateStopped || application.Cmd != nil {
		t.Fatalf("after Stop: state = %q, cmd = %v", application.GetState(), application.Cmd)
	}
}

// 补充测试：验证大锁机制下的并发 EnsureRunning 调用的安全性与正确性
func TestConcurrentEnsureRunning(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required for the process flow test")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	application := NewApp(config.AppConfig{
		Name:         "demo",
		DomainSuffix: ".l.h",
		Cwd:          t.TempDir(),
		Cmd:          "python3",
		Args:         []string{"-m", "http.server", "$PORT"},
	}, port)

	const concurrency = 10
	var wg sync.WaitGroup
	errCh := make(chan error, concurrency)

	// 模拟 10 个并发请求同时激活应用
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if err := application.Run(ctx); err != nil {
				errCh <- err
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent EnsureRunning error: %v", err)
	}

	if application.GetState() != StateRunning {
		t.Fatalf("final state = %q, want %q", application.GetState(), StateRunning)
	}

	application.Stop()
}
