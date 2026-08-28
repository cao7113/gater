package proxy

import (
	"testing"

	"github.com/cao7113/gater/internal/config"
)

func TestAppName(t *testing.T) {
	suffixes := config.DefaultSuffixes
	tests := map[string]string{
		// .l.h 后缀
		"demo.l.h":      "demo",
		"demo.l.h:8080": "demo",
		// .l.s 后缀
		"demo.l.s":      "demo",
		"demo.l.s:8080": "demo",
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
