package api

import "net/http"

func NewHandler(mgr appManager) http.Handler {
	h := &handler{mgr: mgr}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/fs/pick-folder", h.pickFolder)
	mux.HandleFunc("GET /api/apps", h.listApps)
	mux.HandleFunc("POST /api/apps", h.createApp)
	mux.HandleFunc("GET /api/apps/{name}", h.getApp)
	mux.HandleFunc("DELETE /api/apps/{name}", h.deleteApp)
	mux.HandleFunc("POST /api/apps/{name}/stop", h.stopApp)
	mux.HandleFunc("POST /api/apps/{name}/start", h.startApp)
	mux.HandleFunc("GET /api/apps/{name}/logs", h.getLogs)
	return mux
}
