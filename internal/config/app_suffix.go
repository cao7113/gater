package config

import "strings"

// AppSuffix 定义应用域名后缀及对应的访问协议。
type AppSuffix struct {
	Suffix string `yaml:"suffix" json:"suffix"`
	Scheme string `yaml:"scheme" json:"scheme"`
}

// DefaultSuffixes 是默认的应用域名后缀及协议列表。
var DefaultSuffixes = []AppSuffix{
	{Suffix: ".s", Scheme: "https"},
	{Suffix: ".l", Scheme: "http"},
}

// AppSuffixes 是当前进程接受的应用域名后缀列表。
var AppSuffixes = append([]AppSuffix(nil), DefaultSuffixes...)

// IsTargetHost 判断 Host 是否匹配允许的应用域名后缀。
func IsTargetHost(host string) bool {
	for _, suffix := range AppSuffixes {
		if strings.HasSuffix(host, suffix.Suffix) && len(host) > len(suffix.Suffix) {
			return true
		}
	}
	return false
}

// ParseAppSuffix 从字符串解析 AppSuffix（支持 "https:.s", ".s:https", ".s" 等格式）。
func ParseAppSuffix(raw string) AppSuffix {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "https:") || strings.HasPrefix(raw, "https://") {
		s := strings.TrimPrefix(strings.TrimPrefix(raw, "https://"), "https:")
		return AppSuffix{Suffix: s, Scheme: "https"}
	}
	if strings.HasPrefix(raw, "http:") || strings.HasPrefix(raw, "http://") {
		s := strings.TrimPrefix(strings.TrimPrefix(raw, "http://"), "http:")
		return AppSuffix{Suffix: s, Scheme: "http"}
	}
	if parts := strings.Split(raw, ":"); len(parts) == 2 {
		if parts[1] == "https" || parts[1] == "http" {
			return AppSuffix{Suffix: parts[0], Scheme: parts[1]}
		}
		if parts[0] == "https" || parts[0] == "http" {
			return AppSuffix{Suffix: parts[1], Scheme: parts[0]}
		}
	}
	scheme := "http"
	if strings.HasSuffix(raw, ".s") {
		scheme = "https"
	}
	return AppSuffix{Suffix: raw, Scheme: scheme}
}
