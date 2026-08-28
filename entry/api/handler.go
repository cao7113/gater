package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/cao7113/gater/internal/app"
	"github.com/cao7113/gater/internal/config"
	"github.com/cao7113/gater/internal/manager"
	"gopkg.in/yaml.v3"
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
	if cfg, err := config.LoadFrom(filepath.Join(path, "app.yaml")); err == nil {
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

func (h *handler) getStoreConfig(w http.ResponseWriter, _ *http.Request) {
	content, err := h.mgr.StoreConfig()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取 store 配置失败: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

func (h *handler) getConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.mgr.ServerConfig())
}

func (h *handler) createAppFromYAMLFile(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	if strings.TrimSpace(request.Path) == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	if err := h.mgr.AddOrUpdateApp(request.Path); err != nil {
		if errors.Is(err, manager.ErrAppExists) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (h *handler) createAppFromConfig(w http.ResponseWriter, r *http.Request) {
	var cfg config.AppConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid app config")
		return
	}
	if err := config.Validate(cfg); err != nil {
		writeError(w, http.StatusBadRequest, "应用配置无效: "+err.Error())
		return
	}
	if err := config.ValidateDomainSuffix(cfg.DomainSuffix, h.mgr.AppSuffixes()); err != nil {
		writeError(w, http.StatusBadRequest, "应用配置无效: "+err.Error())
		return
	}
	if err := h.mgr.RegisterApp(cfg); err != nil {
		if errors.Is(err, manager.ErrAppExists) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (h *handler) getApp(w http.ResponseWriter, r *http.Request) {
	name := pathName(r)
	application, exists := h.mgr.GetApp(name)
	if !exists {
		writeError(w, http.StatusNotFound, fmt.Sprintf("应用 [%s] 不存在", name))
		return
	}
	writeJSON(w, http.StatusOK, appToInfo(application))
}

func (h *handler) updateApp(w http.ResponseWriter, r *http.Request) {
	name := pathName(r)
	var cfg config.AppConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid app config")
		return
	}
	cfg.Name = name
	if err := config.ValidateDomainSuffix(cfg.DomainSuffix, h.mgr.AppSuffixes()); err != nil {
		writeError(w, http.StatusBadRequest, "应用配置无效: "+err.Error())
		return
	}
	if err := h.mgr.UpdateApp(name, cfg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *handler) getAppConfig(w http.ResponseWriter, r *http.Request) {
	name := pathName(r)
	application, exists := h.mgr.GetApp(name)
	if !exists {
		writeError(w, http.StatusNotFound, fmt.Sprintf("应用 [%s] 不存在", name))
		return
	}
	yamlData, err := yaml.Marshal(application.Config)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "生成 YAML 配置失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"yaml":  string(yamlData),
		"shell": shellCommand(application.Config, application.Port),
	})
}

func shellCommand(cfg config.AppConfig, port int) string {
	args := make([]string, 0, len(cfg.Args))
	for _, arg := range cfg.Args {
		args = append(args, shellQuote(strings.ReplaceAll(arg, "$PORT", fmt.Sprintf("%d", port))))
	}
	envNames := make([]string, 0, len(cfg.Env)+1)
	for name := range cfg.Env {
		if name == "PORT" {
			continue
		}
		envNames = append(envNames, name)
	}
	envNames = append(envNames, "PORT")
	sort.Strings(envNames)
	envParts := make([]string, 0, len(envNames))
	for _, name := range envNames {
		value := cfg.Env[name]
		if name == "PORT" {
			value = fmt.Sprintf("%d", port)
		}
		envParts = append(envParts, shellQuote(name+"="+value))
	}
	parts := append([]string{"cd", "--", shellQuote(cfg.Cwd), "&&", "env"}, envParts...)
	parts = append(parts, shellQuote(cfg.Cmd))
	parts = append(parts, args...)
	return strings.Join(parts, " ")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
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
	return AppInfo{
		Name:             a.Config.Name,
		DomainSuffix:     a.Config.DomainSuffix,
		URL:              a.URL(),
		AppType:          a.Config.AppType,
		Cwd:              a.Config.Cwd,
		Cmd:              a.Config.Cmd,
		Args:             a.Config.Args,
		Env:              a.Config.Env,
		Port:             a.Port,
		State:            string(a.State),
		IdleTimeoutSec:   int(a.Timeout.Seconds()),
		RemainingSeconds: remaining,
		StartupMs:        a.StartupMs,
		LastStartedAt:    a.LastStartedAt,
	}
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
