package entry

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cao7113/gater/internal/config"
)

func TestRoute(t *testing.T) {
	admin := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	remote := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusAccepted) })
	originalHosts := config.AdminHosts
	config.SetAdminHosts("admin.example,admin.lab")
	defer func() { config.AdminHosts = originalHosts }()
	originalSuffixes := config.AppSuffixes
	config.AppSuffixes = []config.AppSuffix{{Suffix: ".lab", Scheme: "http"}}
	defer func() { config.AppSuffixes = originalSuffixes }()
	handler := route(admin, remote)

	tests := []struct {
		name string
		host string
		want int
	}{
		{name: "configured admin host", host: "admin.example:8080", want: http.StatusTeapot},
		{name: "second configured admin host", host: "admin.lab", want: http.StatusTeapot},
		{name: "remote app", host: "demo.lab:8080", want: http.StatusAccepted},
		{name: "unrelated host", host: "example.com:8080", want: http.StatusNotFound},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.Host = test.host
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("want %d, got %d", test.want, response.Code)
			}
		})
	}
}
