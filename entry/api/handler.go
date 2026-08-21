package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/cao7113/gater/internal/app"
	"github.com/cao7113/gater/internal/config"
	"github.com/cao7113/gater/internal/store"
)

type handler struct{ mgr appManager }

func (h *handler) pickFolder(w http.ResponseWriter, _ *http.Request) {
	output, err := exec.Command("osascript", "-e", `POSIX path of (choose folder with prompt "请选择应用项目所在目录")`).Output()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"canceled": true})
		return
	}

	path := strings.TrimRight(strings.TrimSpace(string(output)), "/")
	response := map[string]any{"path": path, "canceled": false}
	if cfg, _, err := config.LoadAppConfig(path); err == nil {
		response["config"] = cfg
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *handler) listApps(w http.ResponseWriter, _ *http.Request) {
	apps := h.mgr.GetAllApps()
	items := make([]AppInfo, 0, len(apps))
	for _, application := range apps {
		items = append(items, appToInfo(application))
	}
	sortByName(items)
	writeJSON(w, http.StatusOK, items)
}

func (h *handler) createApp(w http.ResponseWriter, r *http.Request) {
	var spec store.AppSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if err := h.mgr.AddOrUpdateApp(spec); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (h *handler) deleteApp(w http.ResponseWriter, r *http.Request) {
	name := pathName(r)
	if err := h.mgr.RemoveApp(name); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove app: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *handler) stopApp(w http.ResponseWriter, r *http.Request) {
	name := pathName(r)
	application, exists := h.mgr.GetApp(name)
	if !exists {
		writeError(w, http.StatusNotFound, fmt.Sprintf("应用 [%s] 不存在", name))
		return
	}
	application.Stop()
	w.WriteHeader(http.StatusOK)
}

func (h *handler) startApp(w http.ResponseWriter, r *http.Request) {
	name := pathName(r)
	application, exists := h.mgr.GetApp(name)
	if !exists {
		writeError(w, http.StatusNotFound, fmt.Sprintf("应用 [%s] 不存在", name))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	if err := application.EnsureStarted(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *handler) getLogs(w http.ResponseWriter, r *http.Request) {
	name := pathName(r)
	application, exists := h.mgr.GetApp(name)
	if !exists {
		writeError(w, http.StatusNotFound, fmt.Sprintf("应用 [%s] 不存在", name))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"name": name, "logs": application.LogBuf.String()})
}

func appToInfo(a *app.App) AppInfo {
	remaining := 0
	if a.State == "running" {
		remainingTime := a.Timeout - time.Since(a.LastActive)
		if remainingTime > 0 {
			remaining = int(remainingTime.Seconds())
		}
	}
	return AppInfo{Name: a.Spec.Name, Path: a.Spec.Path, Cmd: a.Spec.Cmd, Args: a.Spec.Args, Env: a.Spec.Env, Port: a.Port, State: string(a.State), IdleTimeoutSec: int(a.Timeout.Seconds()), RemainingSeconds: remaining}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func pathName(r *http.Request) string {
	raw := r.PathValue("name")
	if decoded, err := url.PathUnescape(raw); err == nil {
		return decoded
	}
	return raw
}

func sortByName(apps []AppInfo) {
	sort.Slice(apps, func(i, j int) bool { return apps[i].Name < apps[j].Name })
}
