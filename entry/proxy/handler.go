package proxy

import (
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"

	"github.com/cao7113/gater/internal/config"
	"github.com/cao7113/gater/internal/manager"
)

func init() {
	// 确保 DefaultSuffixes 长后缀优先
	sortSuffixes(config.DefaultSuffixes)
}

// sortSuffixes 就地按后缀长度降序排列。
func sortSuffixes(s []config.AppSuffix) {
	sort.Slice(s, func(i, j int) bool {
		return len(s[i].Suffix) > len(s[j].Suffix)
	})
}

func NewHandler(mgr *manager.Manager, suffixes []config.AppSuffix) http.Handler {
	// 复制一份并排序，避免修改调用方传入的切片
	sorted := make([]config.AppSuffix, len(suffixes))
	copy(sorted, suffixes)
	sortSuffixes(sorted)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := appName(r.Host, sorted)
		app, ok := mgr.GetApp(name)
		if !ok {
			http.Error(w, fmt.Sprintf("Gater: 未注册的应用域名 [%s]", r.Host), http.StatusNotFound)
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

// appName 从 host 中去掉端口后，依次尝试剥离 suffixes 中的后缀，返回应用名。
// 若没有匹配到任何后缀，则返回原始 host（不做截断）。
func appName(host string, suffixes []config.AppSuffix) string {
	host = hostName(host)
	for _, s := range suffixes {
		if strings.HasSuffix(host, s.Suffix) {
			return strings.TrimSuffix(host, s.Suffix)
		}
	}
	return host
}

func hostName(host string) string {
	if name, _, err := net.SplitHostPort(host); err == nil {
		return name
	}
	return strings.Trim(host, "[]")
}
