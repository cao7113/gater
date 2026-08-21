package entry

import (
	"net"
	"net/http"
	"strings"

	"github.com/cao7113/gater/entry/api"
	"github.com/cao7113/gater/entry/proxy"
	"github.com/cao7113/gater/internal/manager"
	"github.com/cao7113/gater/web"
)

type Config struct {
	Port      string
	AdminHost string
}

func New(cfg Config, mgr *manager.Manager) *http.Server {
	adminHandler := http.NewServeMux()

	apiHandler := api.NewHandler(mgr)
	adminHandler.Handle("/api/", apiHandler)
	adminHandler.Handle("/", http.FileServer(web.GetFileSystem()))

	return &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: route(cfg.AdminHost, adminHandler, proxy.NewHandler(mgr)),
	}
}

func route(adminHost string, admin, remote http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := hostName(r.Host)
		if host == adminHost || host == "localhost" || host == "127.0.0.1" {
			admin.ServeHTTP(w, r)
			return
		}
		remote.ServeHTTP(w, r)
	})
}

func hostName(host string) string {
	if name, _, err := net.SplitHostPort(host); err == nil {
		return name
	}
	return strings.Trim(host, "[]")
}
