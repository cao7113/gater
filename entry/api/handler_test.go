package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cao7113/gater/internal/app"
	"github.com/cao7113/gater/internal/store"
)

type fakeMgr struct{ apps map[string]*app.App }

func newFakeMgr(apps ...*app.App) *fakeMgr {
	manager := &fakeMgr{apps: make(map[string]*app.App)}
	for _, application := range apps {
		manager.apps[application.Spec.Name] = application
	}
	return manager
}

func (f *fakeMgr) GetAllApps() map[string]*app.App { return f.apps }
func (f *fakeMgr) GetApp(name string) (*app.App, bool) {
	application, ok := f.apps[name]
	return application, ok
}
func (f *fakeMgr) AddOrUpdateApp(store.AppSpec) error { return nil }
func (f *fakeMgr) RemoveApp(name string) error        { delete(f.apps, name); return nil }

func testApp(name string) *app.App {
	return app.NewApp(store.AppSpec{Name: name, Cmd: "echo", Args: []string{"hello"}}, 59001)
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
