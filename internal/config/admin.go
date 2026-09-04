package config

import "strings"

// DefaultAdminHosts 是管理控制台默认接受的 Host 列表。
var DefaultAdminHosts = []string{
	"a.s", "a.l", "admin.s", "admin.l", "localhost"}

// AdminHosts 是当前进程接受管理请求的 Host 列表。
var AdminHosts = append([]string(nil), DefaultAdminHosts...)

// SetAdminHosts 根据逗号分隔的配置更新管理 Host 列表。
// 空值会恢复默认列表。
func SetAdminHosts(raw string) {
	var hosts []string
	for _, host := range strings.Split(raw, ",") {
		if host = strings.TrimSpace(host); host != "" && !contains(hosts, host) {
			hosts = append(hosts, host)
		}
	}
	if len(hosts) == 0 {
		hosts = append([]string(nil), DefaultAdminHosts...)
	}
	AdminHosts = hosts
}

func IsAdminHost(host string) bool {
	for _, adminHost := range AdminHosts {
		if host == adminHost {
			return true
		}
	}
	return false
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
