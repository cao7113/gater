package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cao7113/gater/internal/app"
	"github.com/cao7113/gater/internal/config"
)

type fakeMgr struct{ apps map[string]*app.App }

func newFakeMgr(apps ...*app.App) *fakeMgr {
	manager := &fakeMgr{apps: make(map[string]*app.App)}
	for _, application := range apps {
		manager.apps[application.Config.Name] = application
	}
	return manager
}

func (f *fakeMgr) GetAllApps() map[string]*app.App { return f.apps }
func (f *fakeMgr) GetApp(name string) (*app.App, bool) {
	application, ok := f.apps[name]
	return application, ok
}
func (f *fakeMgr) AddOrUpdateApp(string) error { return nil }
func (f *fakeMgr) RegisterApp(cfg config.AppConfig) error {
	if cfg.Name == "" {
		return fmt.Errorf("missing name")
	}
	f.apps[cfg.Name] = app.NewApp(cfg, 59001)
	return nil
}
func (f *fakeMgr) UpdateApp(name string, cfg config.AppConfig) error {
	if _, ok := f.apps[name]; !ok {
		return fmt.Errorf("not found")
	}
	cfg.Name = name
	f.apps[name] = app.NewApp(cfg, f.apps[name].Port)
	return nil
}
func (f *fakeMgr) RemoveApp(name string) error  { delete(f.apps, name); return nil }
func (f *fakeMgr) StoreConfig() ([]byte, error) { return []byte("demo:\n  name: demo\n"), nil }
func (f *fakeMgr) AppSuffixes() []config.AppSuffix {
	return []config.AppSuffix{
		{Suffix: ".l.h", Scheme: "http"},
		{Suffix: ".l.s", Scheme: "https"},
	}
}
func (f *fakeMgr) ServerConfig() ServerConfig {
	return ServerConfig{Port: "8080", AdminHost: "admin.lab", StorePath: "~/.config/gater/store.yaml", AppSuffixes: f.AppSuffixes(), AppTemplates: config.DefaultAppTemplates}
}

func testApp(name string) *app.App {
	return app.NewApp(config.AppConfig{Name: name, Cmd: "echo", Args: []string{"hello"}}, 59001)
}

func TestListAppsEmpty(t *testing.T) {
	res := httptest.NewRecorder()
	(&handler{mgr: newFakeMgr()}).listApps(res, httptest.NewRequest(http.MethodGet, "/api/apps", nil))
	var apps []AppInfo
	if err := json.NewDecoder(res.Body).Decode(&apps); err != nil {
		t.Fatal(err)
	}
	if len(apps) != 0 {
		t.Fatalf("want 0 apps, got %d", len(apps))
	}
}

func TestGetConfig(t *testing.T) {
	res := httptest.NewRecorder()
	(&handler{mgr: newFakeMgr()}).getConfig(res, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", res.Code)
	}
	if got := res.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("unexpected content type: %q", got)
	}
	var info ServerConfig
	if err := json.NewDecoder(res.Body).Decode(&info); err != nil {
		t.Fatalf("decode ServerConfig error: %v", err)
	}
	if info.Port != "8080" || info.AdminHost != "admin.lab" || info.StorePath != "~/.config/gater/store.yaml" || len(info.AppSuffixes) != 2 {
		t.Fatalf("unexpected ServerConfig: %+v", info)
	}
	if len(info.AppTemplates) != 3 || info.AppTemplates[0].ID != config.AppTypePhx || info.AppTemplates[1].ID != "bun" || info.AppTemplates[2].ID != "python" {
		t.Fatalf("unexpected app templates: %+v", info.AppTemplates)
	}
}

func TestCreateAppFromConfig(t *testing.T) {
	mgr := newFakeMgr()
	req := httptest.NewRequest(http.MethodPost, "/api/apps/from-config", bytes.NewBufferString(`{
		"name": "phoenix-demo",
		"domain_suffix": ".l.h",
		"app_type": "phx",
		"cwd": "/tmp/phoenix-demo",
		"cmd": "mix",
		"args": ["phx.server"],
		"idle_timeout": "10m"
	}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	(&handler{mgr: mgr}).createAppFromConfig(res, req)

	if res.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", res.Code, res.Body.String())
	}
	application, ok := mgr.GetApp("phoenix-demo")
	if !ok {
		t.Fatal("application was not registered")
	}
	if application.Config.AppType != config.AppTypePhx || application.Config.Cmd != "mix" || application.Config.Cwd != "/tmp/phoenix-demo" {
		t.Fatalf("unexpected registered config: %+v", application.Config)
	}
	if application.Config.DomainSuffix != ".l.h" {
		t.Fatalf("unexpected domain suffix: %q", application.Config.DomainSuffix)
	}
}

func TestCreateAppFromConfigRejectsDisallowedSuffix(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/apps/from-config", strings.NewReader(`{
		"name":"demo","domain_suffix":".invalid","cwd":"/tmp/demo","cmd":"mix","idle_timeout":"10m"
	}`))
	res := httptest.NewRecorder()
	(&handler{mgr: newFakeMgr()}).createAppFromConfig(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", res.Code)
	}
}

func TestCreateAppFromConfigRequiresCwd(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/apps/from-config", strings.NewReader(`{"name":"demo","cmd":"mix","idle_timeout":"10m"}`))
	res := httptest.NewRecorder()
	(&handler{mgr: newFakeMgr()}).createAppFromConfig(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", res.Code)
	}
}

func TestGetStoreConfig(t *testing.T) {
	res := httptest.NewRecorder()
	(&handler{mgr: newFakeMgr()}).getStoreConfig(res, httptest.NewRequest(http.MethodGet, "/api/store/config", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", res.Code)
	}
	if got := res.Body.String(); got != "demo:\n  name: demo\n" {
		t.Fatalf("unexpected config: %q", got)
	}
	if got := res.Header().Get("Content-Type"); got != "text/yaml; charset=utf-8" {
		t.Fatalf("unexpected content type: %q", got)
	}
}

func TestListAppsSorted(t *testing.T) {
	res := httptest.NewRecorder()
	(&handler{mgr: newFakeMgr(testApp("zebra"), testApp("alpha"))}).listApps(res, httptest.NewRequest(http.MethodGet, "/api/apps", nil))
	var apps []AppInfo
	if err := json.NewDecoder(res.Body).Decode(&apps); err != nil {
		t.Fatal(err)
	}
	if apps[0].Name != "alpha" || apps[1].Name != "zebra" {
		t.Fatalf("want sorted apps")
	}
}

func TestListAppsRemainingSeconds(t *testing.T) {
	application := testApp("myapp")
	application.State = "running"
	application.Timeout = 5 * time.Minute
	application.LastActive = time.Now().Add(-10 * time.Second)
	res := httptest.NewRecorder()
	(&handler{mgr: newFakeMgr(application)}).listApps(res, httptest.NewRequest(http.MethodGet, "/api/apps", nil))
	var apps []AppInfo
	if err := json.NewDecoder(res.Body).Decode(&apps); err != nil {
		t.Fatal(err)
	}
	if apps[0].RemainingSeconds < 288 || apps[0].RemainingSeconds > 292 {
		t.Fatalf("want ~290s remaining, got %d", apps[0].RemainingSeconds)
	}
}

func TestDeleteApp(t *testing.T) {
	req := httptest.NewRequest(http.MethodDelete, "/api/apps/myapp", nil)
	req.SetPathValue("name", "myapp")
	res := httptest.NewRecorder()
	(&handler{mgr: newFakeMgr(testApp("myapp"))}).deleteApp(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", res.Code)
	}
}

func TestGetApp(t *testing.T) {
	h := &handler{mgr: newFakeMgr(testApp("myapp"))}
	req := httptest.NewRequest(http.MethodGet, "/api/apps/myapp", nil)
	req.SetPathValue("name", "myapp")
	res := httptest.NewRecorder()

	h.getApp(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", res.Code)
	}
	var app AppInfo
	if err := json.NewDecoder(res.Body).Decode(&app); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if app.Name != "myapp" {
		t.Fatalf("want myapp, got %s", app.Name)
	}
}

func TestListAppsStartupInfo(t *testing.T) {
	application := testApp("myapp")
	startedAt := time.Date(2026, time.August, 27, 12, 34, 56, 0, time.UTC)
	application.StartupMs = 123
	application.LastStartedAt = &startedAt

	res := httptest.NewRecorder()
	(&handler{mgr: newFakeMgr(application)}).listApps(res, httptest.NewRequest(http.MethodGet, "/api/apps", nil))

	var apps []AppInfo
	if err := json.NewDecoder(res.Body).Decode(&apps); err != nil {
		t.Fatal(err)
	}
	if apps[0].StartupMs != 123 {
		t.Fatalf("want startup time 123ms, got %d", apps[0].StartupMs)
	}
	if apps[0].LastStartedAt == nil || !apps[0].LastStartedAt.Equal(startedAt) {
		t.Fatalf("want last started at %s, got %s", startedAt, apps[0].LastStartedAt)
	}
}

func TestGetAppConfigIncludesYAMLAndShell(t *testing.T) {
	application := app.NewApp(config.AppConfig{
		Name:         "myapp",
		DomainSuffix: ".l.h",
		Cwd:          "/tmp/my app's dir",
		Cmd:          "sh",
		Args:         []string{"-c", "python3 -m http.server $PORT"},
		Env:          map[string]string{"FOO": "bar baz", "PORT": "ignored"},
		IdleTimeout:  "10m",
	}, 59001)
	req := httptest.NewRequest(http.MethodGet, "/api/apps/myapp/config", nil)
	req.SetPathValue("name", "myapp")
	res := httptest.NewRecorder()
	(&handler{mgr: newFakeMgr(application)}).getAppConfig(res, req)

	var result map[string]string
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result["yaml"], "name: myapp") {
		t.Fatalf("YAML does not contain app name: %q", result["yaml"])
	}
	wantShell := "cd -- '/tmp/my app'\\''s dir' && env 'FOO=bar baz' 'PORT=59001' 'sh' '-c' 'python3 -m http.server 59001'"
	if result["shell"] != wantShell {
		t.Fatalf("unexpected shell command:\nwant: %s\ngot:  %s", wantShell, result["shell"])
	}
}
