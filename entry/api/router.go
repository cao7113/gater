package api

import "net/http"

func NewHandler(mgr appManager) http.Handler {
	h := &handler{mgr: mgr}
	mux := http.NewServeMux()

	// File selection and YAML import.
	mux.HandleFunc("POST /api/fs/pick-yaml-file", h.pickYAMLFile)
	mux.HandleFunc("POST /api/apps/from-yaml", h.fromYAML)

	// Server and store configuration.
	mux.HandleFunc("GET /api/config", h.getConfig)
	mux.HandleFunc("GET /api/store/config", h.getStoreConfig)

	// App collection operations.
	mux.HandleFunc("GET /api/apps", h.listApps)
	mux.HandleFunc("POST /api/apps/from-config", h.createAppFromConfig)

	// Single app operations.
	mux.HandleFunc("GET /api/apps/{name}", h.getApp)
	mux.HandleFunc("PUT /api/apps/{name}", h.updateApp)
	mux.HandleFunc("GET /api/apps/{name}/config", h.getAppConfig)
	mux.HandleFunc("DELETE /api/apps/{name}", h.deleteApp)
	mux.HandleFunc("POST /api/apps/{name}/stop", h.stopApp)
	mux.HandleFunc("POST /api/apps/{name}/start", h.startApp)
	mux.HandleFunc("GET /api/apps/{name}/logs", h.getLogs)
	return mux
}
