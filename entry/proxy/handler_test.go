package proxy

import "testing"

func TestAppName(t *testing.T) {
	tests := map[string]string{
		"demo.lab":      "demo",
		"demo.lab:8080": "demo",
		"[::1]:8080":    "::1",
		"admin.example": "admin.example",
	}
	for host, want := range tests {
		if got := appName(host); got != want {
			t.Errorf("appName(%q) = %q, want %q", host, got, want)
		}
	}
}
