package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
)

// NewProxyErrorHandler 独立出来的代理异常处理器
func NewProxyErrorHandler(appName string) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		// 1. 忽略客户端主动取消/断开连接的噪音日志
		if errors.Is(err, context.Canceled) || errors.Is(r.Context().Err(), context.Canceled) {
			log.Printf("[Gater] [%s] 客户端主动中断连接: %s %s", appName, r.Method, r.URL.Path)
			w.WriteHeader(499) // 499 Client Closed Request
			return
		}

		// 2. 真实的后端服务异常
		log.Printf("[Gater] [%s] 代理转发异常: %v", appName, err)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("502 Bad Gateway - Application is unreachable or restarting"))
	}
}

// ExpandPlaceholders 使用 vars 字典展开字符串中的 ${VAR} 或 $VAR 占位符
func ExpandPlaceholders(s string, vars map[string]string) string {
	return os.Expand(s, func(k string) string {
		if v, ok := vars[k]; ok {
			return v
		}
		return os.Getenv(k) // 保留系统环境变量
	})
}

// ExpandSlice 批量展开切片中的占位符
func ExpandSlice(slice []string, vars map[string]string) []string {
	result := make([]string, len(slice))
	for i, v := range slice {
		result[i] = ExpandPlaceholders(v, vars)
	}
	return result
}

func ToEnvList(envMap map[string]string) []string {
	envList := make([]string, 0, len(envMap))
	for k, v := range envMap {
		envList = append(envList, fmt.Sprintf("%s=%s", k, v))
	}
	return envList
}

func cloneEnvMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

var defaultSensitiveEnvKeywords = []string{
	"secret",
	"token",
	"password",
	"passwd",
	"cookie",
	"auth",
	"private",
	"key",
}

func redactEnvValue(name, value string, sensitiveKeywords ...string) string {
	if value == "" {
		return value
	}

	keywords := defaultSensitiveEnvKeywords
	if len(sensitiveKeywords) > 0 {
		keywords = sensitiveKeywords
	}

	key := strings.ToLower(name)
	for _, keyword := range keywords {
		if strings.Contains(key, strings.ToLower(keyword)) {
			return "***redacted***"
		}
	}

	return value
}

func sanitizeEnvMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = redactEnvValue(k, v)
	}
	return out
}
