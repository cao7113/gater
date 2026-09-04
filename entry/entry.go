package entry

import (
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/cao7113/gater/entry/api"
	"github.com/cao7113/gater/entry/proxy"
	"github.com/cao7113/gater/internal/config"
	"github.com/cao7113/gater/internal/manager"
	"github.com/cao7113/gater/internal/version"
	"github.com/cao7113/gater/web"
)

type Config struct {
	AdminPort   string
	StorePath   string
	TargetHost  string
	AppSuffixes []config.AppSuffix
}

func New(cfg Config, mgr *manager.Manager) *http.Server {
	if strings.TrimSpace(cfg.TargetHost) != "" {
		config.TargetHost = strings.TrimSpace(cfg.TargetHost)
	}
	adminHandler := http.NewServeMux()

	suffixes := cfg.AppSuffixes
	if len(suffixes) == 0 {
		suffixes = config.DefaultSuffixes
	}
	config.AppSuffixes = append([]config.AppSuffix(nil), suffixes...)

	apiHandler := api.NewHandler(&serverCtx{Manager: mgr, cfg: cfg, appSuffixes: suffixes})
	adminHandler.Handle("/api/", apiHandler)
	adminHandler.Handle("/", http.FileServer(web.GetFileSystem()))

	return &http.Server{
		Addr:    ":" + cfg.AdminPort,
		Handler: route(adminHandler, proxy.NewHandler(mgr, suffixes)),
	}
}

func route(admin, remote http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := hostName(r.Host)

		if config.IsAdminHost(host) {
			// log.Printf("admin request: %s", host)
			admin.ServeHTTP(w, r)
			return
		}

		if config.IsTargetHost(host) {
			// log.Printf("remote request: %s", host)
			remote.ServeHTTP(w, r)
			return
		}

		// 未匹配到管理域名或目标应用域名的流量直接忽略。
		log.Printf("[Gater] 忽略未匹配域名请求: host=%s method=%s path=%s", host, r.Method, r.URL.Path)
		// Not available for this host, return 404 Not Found.
		http.NotFound(w, r)
	})
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
		Version:      version.String(),
		AdminPort:    s.cfg.AdminPort,
		AdminHost:    config.AdminHosts[0],
		TargetHost:   config.TargetHost,
		StorePath:    s.cfg.StorePath,
		AppSuffixes:  s.appSuffixes,
		AppTemplates: config.DefaultAppTemplates,
	}
}

func hostName(host string) string {
	if name, _, err := net.SplitHostPort(host); err == nil {
		return name
	}
	return strings.Trim(host, "[]")
}
