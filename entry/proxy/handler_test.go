package proxy

import (
	"testing"

	"github.com/cao7113/gater/internal/config"
)

func TestAppName(t *testing.T) {
	suffixes := config.DefaultSuffixes
	tests := map[string]string{
		// .l 后缀
		"demo.l":      "demo",
		"demo.l:8080": "demo",
		// .s 后缀
		"demo.s":      "demo",
		"demo.s:8080": "demo",
		// IPv6
		"[::1]:8080": "::1",
		// 无匹配后缀，原样返回
		"admin.example": "admin.example",
	}
	for host, want := range tests {
		if got := appName(host, suffixes); got != want {
			t.Errorf("appName(%q) = %q, want %q", host, got, want)
		}
	}
}
