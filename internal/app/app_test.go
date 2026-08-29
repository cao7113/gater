package app

import (
	"context"
	"net"
	"os/exec"
	"strings"
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

func TestEnvironmentInjectsPhxHost(t *testing.T) {
	application := NewApp(config.AppConfig{
		Name:         "demo",
		DomainSuffix: ".l.s",
		AppType:      config.AppTypePhoenix,
		Env:          map[string]string{},
	}, 50001)

	values := make(map[string]string)
	appTypeContext := application.newAppTypeContext()
	if err := types.HandlerFor(application.Config.AppType).Prepare(appTypeContext); err != nil {
		t.Fatal(err)
	}
	for _, item := range envList(appTypeContext.Env) {
		parts := strings.SplitN(item, "=", 2)
		values[parts[0]] = parts[1]
	}

	if values["PHX_HOST"] != "demo.l.s" {
		t.Fatalf("PHX_HOST = %q, want demo.l.s", values["PHX_HOST"])
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

func TestAppTypeHandlerFor(t *testing.T) {
	appTypeContext := &types.TypeContext{
		Config: config.AppConfig{AppType: config.AppTypePhoenix},
		Domain: "demo.l.h",
		Env:    map[string]string{},
	}
	if err := types.HandlerFor(config.AppTypePhoenix).Prepare(appTypeContext); err != nil {
		t.Fatal(err)
	}
	if appTypeContext.Env["PHX_HOST"] != "demo.l.h" {
		t.Fatalf("PHX_HOST = %q, want demo.l.h", appTypeContext.Env["PHX_HOST"])
	}

	defaultContext := &types.TypeContext{Env: map[string]string{}}
	if err := types.HandlerFor("unknown").Prepare(defaultContext); err != nil {
		t.Fatal(err)
	}
	if _, ok := defaultContext.Env["PHX_HOST"]; ok {
		t.Fatal("default handler unexpectedly added PHX_HOST")
	}
}

func TestEnsureRunningRejectsMissingWorkingDirectory(t *testing.T) {
	application := NewApp(config.AppConfig{
		Name:         "demo",
		DomainSuffix: ".l.h",
		Cwd:          "/path/that/does/not/exist",
		Cmd:          "echo",
	}, 50001)

	err := application.EnsureRunning(context.Background())
	if err == nil {
		t.Fatal("missing working directory was accepted")
	}
	if application.State != StateCrashed {
		t.Fatalf("state = %q, want %q", application.State, StateCrashed)
	}
}

func TestEnsureRunningRejectsMissingCommand(t *testing.T) {
	application := NewApp(config.AppConfig{
		Name:         "demo",
		DomainSuffix: ".l.h",
		Cwd:          t.TempDir(),
		Cmd:          "command-that-does-not-exist",
	}, 50001)

	err := application.EnsureRunning(context.Background())
	if err == nil {
		t.Fatal("missing command was accepted")
	}
	if application.State != StateCrashed {
		t.Fatalf("state = %q, want %q", application.State, StateCrashed)
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
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := application.EnsureRunning(ctx); err != nil {
		t.Fatalf("EnsureRunning() error = %v", err)
	}
	if application.State != StateRunning {
		t.Fatalf("state = %q, want %q", application.State, StateRunning)
	}
	application.Stop()
	if application.State != StateStopped || application.Cmd != nil {
		t.Fatalf("after Stop: state = %q, cmd = %v", application.State, application.Cmd)
	}
}
