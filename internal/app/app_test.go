package app

import (
	"context"
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

func TestEnvironment(t *testing.T) {
	application := NewApp(config.AppConfig{
		Name:         "demo",
		DomainSuffix: ".s",
		Port:         50001,
		Env: map[string]string{
			"FOO": "bar", "aPort": "${PORT}"},
	})
	application.Port = application.Config.Port

	values := make(map[string]string)
	if application.Domain() != "demo.s" {
		t.Fatalf("Domain() = %q, want demo.s", application.Domain())
	}
	appTypeContext := application.newAppTypeContext()
	if err := types.HandlerFor(application.Config.AppType).Prepare(appTypeContext); err != nil {
		t.Fatal(err)
	}
	for _, item := range ToEnvList(appTypeContext.Env) {
		parts := strings.SplitN(item, "=", 2)
		values[parts[0]] = parts[1]
	}

	if values["aPort"] != "50001" {
		t.Fatalf("aPort = %q, want 50001", values["aPort"])
	}
	if values["FOO"] != "bar" {
		t.Fatalf("FOO = %q, want bar", values["FOO"])
	}
}

func TestURLUsesAppDomain(t *testing.T) {
	application := NewApp(config.AppConfig{Name: "demo", DomainSuffix: ".l"})
	if got := application.URL(); got != "http://demo.l" {
		t.Fatalf("URL() = %q, want http://demo.l", got)
	}
	if got := application.URL("https"); got != "https://demo.l" {
		t.Fatalf("URL(https) = %q, want https://demo.l", got)
	}
}

func TestSnapshotRedactsSensitiveEnvByDefault(t *testing.T) {
	application := NewApp(config.AppConfig{
		Name: "demo",
		Env: map[string]string{
			"APP_MODE":  "dev",
			"API_TOKEN": "secret-value",
		},
	})
	application.State = StateRunning
	application.RuntimeEnv = map[string]string{
		"APP_MODE":  "dev",
		"API_TOKEN": "secret-value",
	}

	redacted := application.Snapshot()
	redactedEnv := redacted["env"].(map[string]string)
	if redactedEnv["APP_MODE"] != "dev" || redactedEnv["API_TOKEN"] != "***redacted***" {
		t.Fatalf("unexpected redacted environment: %#v", redactedEnv)
	}

	visible := application.SnapshotWithSensitive(true)
	visibleEnv := visible["env"].(map[string]string)
	if visibleEnv["API_TOKEN"] != "secret-value" {
		t.Fatalf("sensitive environment value was not restored: %#v", visibleEnv)
	}
}

func TestSnapshotUsesConfiguredEnvWhenNotRunning(t *testing.T) {
	application := NewApp(config.AppConfig{
		Name: "demo",
		Env:  map[string]string{"APP_MODE": "dev"},
	})

	snapshot := application.Snapshot()
	env := snapshot["env"].(map[string]string)
	if env["APP_MODE"] != "dev" {
		t.Fatalf("configured environment was not used: %#v", env)
	}
}

func TestRunRejectsMissingWorkingDirectory(t *testing.T) {
	application := NewApp(config.AppConfig{
		Name:         "demo",
		DomainSuffix: ".l",
		Cwd:          "/path/that/does/not/exist",
		Cmd:          "echo",
	})

	err := application.Run(context.Background())
	if err == nil {
		t.Fatal("missing working directory was accepted")
	}
	if application.GetState() != StateCrashed {
		t.Fatalf("state = %q, want %q", application.GetState(), StateCrashed)
	}
}

func TestRunRejectsMissingCommand(t *testing.T) {
	application := NewApp(config.AppConfig{
		Name:         "demo",
		DomainSuffix: ".l",
		Cwd:          t.TempDir(),
		Cmd:          "command-that-does-not-exist",
	})

	err := application.Run(context.Background())
	if err == nil {
		t.Fatal("missing command was accepted")
	}
	if application.GetState() != StateCrashed {
		t.Fatalf("state = %q, want %q", application.GetState(), StateCrashed)
	}
}

func TestRunRunsAndStopsApplication(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required for the process flow test")
	}
	port, err := NextPort()
	if err != nil {
		t.Fatal(err)
	}

	application := NewApp(config.AppConfig{
		Name:         "demo",
		DomainSuffix: ".l",
		Cwd:          t.TempDir(),
		Cmd:          "python3",
		Args:         []string{"-m", "http.server", "$PORT"},
		IdleTimeout:  "1m",
		Port:         port,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := application.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if application.GetState() != StateRunning {
		t.Fatalf("state = %q, want %q", application.GetState(), StateRunning)
	}

	// 验证反向代理请求重写功能 (针对 Go 1.18+ Rewrite 接口)
	req := httptest.NewRequest("GET", "http://demo.l/", nil)
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

// 补充测试：验证大锁机制下的并发 Run 调用的安全性与正确性
func TestConcurrentRun(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required for the process flow test")
	}
	port, err := NextPort()
	if err != nil {
		t.Fatal(err)
	}

	application := NewApp(config.AppConfig{
		Name:         "demo",
		DomainSuffix: ".l",
		Cwd:          t.TempDir(),
		Cmd:          "python3",
		Args:         []string{"-m", "http.server", "$PORT"},
		Port:         port,
	})

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
		t.Errorf("concurrent Run error: %v", err)
	}

	if application.GetState() != StateRunning {
		t.Fatalf("final state = %q, want %q", application.GetState(), StateRunning)
	}

	application.Stop()
}
