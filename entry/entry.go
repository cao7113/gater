package entry

import (
	"net"
	"net/http"
	"strings"

	"github.com/cao7113/gater/entry/api"
	"github.com/cao7113/gater/entry/proxy"
	"github.com/cao7113/gater/internal/config"
	"github.com/cao7113/gater/internal/manager"
	"github.com/cao7113/gater/web"
)

type Config struct {
	Port        string
	AdminHost   string
	StorePath   string
	AppSuffixes []config.AppSuffix
}

func New(cfg Config, mgr *manager.Manager) *http.Server {
	adminHandler := http.NewServeMux()

	suffixes := cfg.AppSuffixes
	if len(suffixes) == 0 {
		suffixes = config.DefaultSuffixes
	}

	apiHandler := api.NewHandler(&serverCtx{Manager: mgr, cfg: cfg, appSuffixes: suffixes})
	adminHandler.Handle("/api/", apiHandler)
	adminHandler.Handle("/", http.FileServer(web.GetFileSystem()))

	return &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: route(cfg.AdminHost, adminHandler, proxy.NewHandler(mgr, suffixes)),
	}
}

// serverCtx 在 manager.Manager 基础上注入运行时配置，
// 满足 api.appManager 接口的 AppSuffixes() 和 ServerConfig() 方法。
type serverCtx struct {
	*manager.Manager
	cfg         Config
	appSuffixes []config.AppSuffix
}

func (s *serverCtx) AppSuffixes() []config.AppSuffix { return s.appSuffixes }

func (s *serverCtx) ServerConfig() api.ServerConfig {
	return api.ServerConfig{
		Port:         s.cfg.Port,
		AdminHost:    s.cfg.AdminHost,
		StorePath:    s.cfg.StorePath,
		AppSuffixes:  s.appSuffixes,
		AppTemplates: config.DefaultAppTemplates,
	}
}

func route(adminHost string, admin, remote http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// fmt.Println("# Request ", r.Host)
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
