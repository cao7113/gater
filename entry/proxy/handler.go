package proxy

import (
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/cao7113/gater/internal/manager"
)

func NewHandler(mgr *manager.Manager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := appName(r.Host)
		app, ok := mgr.GetApp(name)
		if !ok {
			http.Error(w, fmt.Sprintf("Gater: 未注册的应用域名 [%s.lab]", name), http.StatusNotFound)
			return
		}
		if err := app.EnsureStarted(r.Context()); err != nil {
			http.Error(w, fmt.Sprintf("Gater: 无法拉起服务 [%s]: %v", name, err), http.StatusBadGateway)
			return
		}
		app.Touch()
		app.Proxy.ServeHTTP(w, r)
	})
}

func appName(host string) string {
	if name, _, err := net.SplitHostPort(host); err == nil {
		host = name
	} else {
		host = strings.Trim(host, "[]")
	}
	return strings.TrimSuffix(host, ".lab")
}
