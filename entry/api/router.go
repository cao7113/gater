package api

import "net/http"

func NewHandler(mgr appManager) http.Handler {
	h := &handler{mgr: mgr}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/fs/pick-folder", h.pickFolder)
	mux.HandleFunc("GET /api/config", h.getConfig)
	mux.HandleFunc("GET /api/store/config", h.getStoreConfig)
	mux.HandleFunc("GET /api/apps", h.listApps)
	mux.HandleFunc("POST /api/apps/from-yaml-file", h.createAppFromYAMLFile)
	mux.HandleFunc("POST /api/apps/from-config", h.createAppFromConfig)
	mux.HandleFunc("GET /api/apps/{name}", h.getApp)
	mux.HandleFunc("PUT /api/apps/{name}", h.updateApp)
	mux.HandleFunc("GET /api/apps/{name}/config", h.getAppConfig)
	mux.HandleFunc("DELETE /api/apps/{name}", h.deleteApp)
	mux.HandleFunc("POST /api/apps/{name}/stop", h.stopApp)
	mux.HandleFunc("POST /api/apps/{name}/start", h.startApp)
	mux.HandleFunc("GET /api/apps/{name}/logs", h.getLogs)
	return mux
}
