package entry

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRoute(t *testing.T) {
	admin := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	remote := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusAccepted) })
	handler := route("admin.example", admin, remote)

	tests := []struct {
		name string
		host string
		want int
	}{
		{name: "configured admin host", host: "admin.example:8080", want: http.StatusTeapot},
		{name: "localhost", host: "localhost", want: http.StatusTeapot},
		{name: "remote app", host: "demo.lab:8080", want: http.StatusAccepted},
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
