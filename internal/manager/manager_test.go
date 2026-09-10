package manager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/cao7113/gater/internal/config"
	"github.com/cao7113/gater/internal/store"
)

func TestAddOrUpdateAppLoadsEnvFromAppConfig(t *testing.T) {
	appDir := t.TempDir()
	appYAML := `
name: demo
domain_suffix: .l
cmd: echo
args:
- foo
- bar
env:
  DEMO_MODE: test
  DEMO_PORT: 1234
idle_timeout: 5m
`
	appFile := filepath.Join(appDir, "app.yaml")
	if err := os.WriteFile(appFile, []byte(appYAML), 0600); err != nil {
		t.Fatal(err)
	}

	storeFile := filepath.Join(t.TempDir(), "store.yaml")
	st, err := store.NewStore(storeFile)
	if err != nil {
		t.Fatal(err)
	}
	mgr := New(context.Background(), st)
	if err := mgr.AddOrUpdateApp(appFile); err != nil {
		t.Fatal(err)
	}

	// content, err := os.ReadFile(storeFile)
	// if err != nil {
	// 	t.Fatal(err)
	// }
	// t.Logf("Store yaml file content:\n%s", content)

	app, ok := mgr.GetApp("demo")
	if !ok {
		t.Fatal("application was not registered")
	}
	if app.Config.Env["DEMO_MODE"] != "test" || app.Config.Env["DEMO_PORT"] != "1234" {
		t.Fatalf("app.yaml environment was not loaded: %#v", app.Config.Env)
	}
	if app.Config.Name != "demo" || app.Config.Cmd != "echo" || app.Config.IdleTimeout != "5m" {
		t.Fatalf("app.yaml defaults were not loaded: %#v", app.Config)
	}
}

func TestAddOrUpdateAppLoadsConfigFromPath(t *testing.T) {
	appDir := t.TempDir()
	appFile := filepath.Join(appDir, "app.yaml")
	if err := os.WriteFile(appFile, []byte("name: requested\ndomain_suffix: .l\ncmd: printf\nargs: [requested]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	st, err := store.NewStore(filepath.Join(t.TempDir(), "store.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	mgr := New(context.Background(), st)
	err = mgr.AddOrUpdateApp(appFile)
	if err != nil {
		t.Fatal(err)
	}

	app, ok := mgr.GetApp("requested")
	if !ok {
		t.Fatal("application was not registered with request name")
	}
	if app.Config.Name != "requested" || app.Config.Cmd != "printf" || len(app.Config.Args) != 1 || app.Config.Args[0] != "requested" {
		t.Fatalf("request config did not override app.yaml: %#v", app.Config)
	}
}

func TestAddOrUpdateAppRejectsDuplicateName(t *testing.T) {
	appDir := t.TempDir()
	appFile := filepath.Join(appDir, "app.yaml")
	if err := os.WriteFile(appFile, []byte("name: demo\ndomain_suffix: .l\ncmd: echo\n"), 0600); err != nil {
		t.Fatal(err)
	}
	st, err := store.NewStore(filepath.Join(t.TempDir(), "store.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	mgr := New(context.Background(), st)
	if err := mgr.AddOrUpdateApp(appFile); err != nil {
		t.Fatal(err)
	}
	if err := mgr.AddOrUpdateApp(appFile); err == nil {
		t.Fatal("duplicate application was accepted")
	}
}

func TestAddOrUpdateAppRejectsInvalidAppConfig(t *testing.T) {
	appDir := t.TempDir()
	appFile := filepath.Join(appDir, "app.yaml")
	if err := os.WriteFile(appFile, []byte("name: demo\ncmd: echo\nidle_timeout: invalid\n"), 0600); err != nil {
		t.Fatal(err)
	}
	st, err := store.NewStore(filepath.Join(t.TempDir(), "store.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	mgr := New(context.Background(), st)
	if err := mgr.AddOrUpdateApp(appFile); err == nil {
		t.Fatal("invalid app.yaml was accepted")
	}
	if _, ok := mgr.GetApp("demo"); ok {
		t.Fatal("invalid app.yaml was registered")
	}
}

func TestRegisterAppRejectsDisallowedSuffix(t *testing.T) {
	st, err := store.NewStore(filepath.Join(t.TempDir(), "store.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	mgr := New(context.Background(), st)
	err = mgr.RegisterApp(config.AppConfig{Name: "demo", DomainSuffix: ".invalid", Cwd: "/tmp", Cmd: "echo", IdleTimeout: "5m"})
	if err == nil {
		t.Fatal("disallowed domain suffix was accepted")
	}
}

func TestAddOrUpdateAppRequiresYAMLPath(t *testing.T) {
	appDir := t.TempDir()
	st, err := store.NewStore(filepath.Join(t.TempDir(), "store.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	mgr := New(context.Background(), st)
	if err := mgr.AddOrUpdateApp(appDir); err == nil {
		t.Fatal("directory path was accepted as app.yaml")
	}
}

func TestAliasesUseUnifiedNamesIndex(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.yaml")
	st, err := store.NewStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	mgr := New(context.Background(), st)

	if err := mgr.RegisterApp(config.AppConfig{
		Name: "livebook", Aliases: []string{"lb", "lv"}, DomainSuffix: ".l", Cwd: "/tmp", Cmd: "echo", IdleTimeout: "5m",
	}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"livebook", "lb", "LV"} {
		if application, ok := mgr.GetApp(name); !ok || application.Config.Name != "livebook" {
			t.Fatalf("GetApp(%q) did not resolve canonical app: %#v, %v", name, application, ok)
		}
	}

	err = mgr.RegisterApp(config.AppConfig{Name: "other", Aliases: []string{"LB"}, DomainSuffix: ".l", Cwd: "/tmp", Cmd: "echo", IdleTimeout: "5m"})
	if !errors.Is(err, ErrAppExists) {
		t.Fatalf("expected alias conflict, got %v", err)
	}

	if err := mgr.UpdateApp("lb", config.AppConfig{
		Aliases: []string{"book"}, DomainSuffix: ".l", Cwd: "/tmp", Cmd: "echo", IdleTimeout: "5m",
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok := mgr.GetApp("lb"); ok {
		t.Fatal("old alias remained after update")
	}
	if application, ok := mgr.GetApp("book"); !ok || application.Config.Name != "livebook" {
		t.Fatal("new alias did not resolve after update")
	}

	if err := mgr.RemoveApp("book"); err != nil {
		t.Fatal(err)
	}
	if _, ok := mgr.GetApp("livebook"); ok {
		t.Fatal("canonical name remained after removal")
	}

	st, err = store.NewStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	mgr = New(context.Background(), st)
	if _, ok := mgr.GetApp("book"); ok {
		t.Fatal("removed alias was restored from store")
	}
}

func TestEndpointUsesUnifiedNamesIndex(t *testing.T) {
	st, err := store.NewStore(filepath.Join(t.TempDir(), "store.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	mgr := New(context.Background(), st)
	if err := mgr.RegisterApp(config.AppConfig{
		Name: "livebook", Aliases: []string{"lb"}, DomainSuffix: ".s", Cwd: "/tmp", Cmd: "echo", IdleTimeout: "5m",
		Endpoints: []config.EndpointConfig{{EntryName: "livebook-iframe", PortEnv: "IFRAME_PORT"}},
	}); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name     string
		endpoint string
	}{
		{"livebook", ""},
		{"LB", ""},
		{"livebook-iframe", "livebook-iframe"},
	} {
		application, endpoint, ok := mgr.GetEntry(test.name)
		if !ok || application.Config.Name != "livebook" || endpoint != test.endpoint {
			t.Fatalf("GetEntry(%q) = (%v, %q, %v)", test.name, application, endpoint, ok)
		}
	}

	err = mgr.RegisterApp(config.AppConfig{
		Name: "other", DomainSuffix: ".s", Cwd: "/tmp", Cmd: "echo", IdleTimeout: "5m",
		Endpoints: []config.EndpointConfig{{EntryName: "LB", PortEnv: "OTHER_PORT"}},
	})
	if !errors.Is(err, ErrAppExists) {
		t.Fatalf("expected endpoint conflict, got %v", err)
	}
}
