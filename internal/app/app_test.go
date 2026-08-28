package app

import (
	"strings"
	"testing"

	"github.com/cao7113/gater/internal/config"
)

func TestEnvironmentInjectsPortAndAppDomain(t *testing.T) {
	application := NewApp(config.AppConfig{
		Name: "demo",
		Env:  map[string]string{"APP_DOMAIN": "user-value", "FOO": "bar"},
	}, 50001)

	values := make(map[string]string)
	for _, item := range application.environment("demo.lab.s") {
		parts := strings.SplitN(item, "=", 2)
		values[parts[0]] = parts[1]
	}

	if values["PORT"] != "50001" {
		t.Fatalf("PORT = %q, want 50001", values["PORT"])
	}
	if values["APP_DOMAIN"] != "demo.lab.s" {
		t.Fatalf("APP_DOMAIN = %q, want demo.lab.s", values["APP_DOMAIN"])
	}
	if values["FOO"] != "bar" {
		t.Fatalf("FOO = %q, want bar", values["FOO"])
	}
}

func TestEnvironmentInjectsPhxHost(t *testing.T) {
	application := NewApp(config.AppConfig{
		Name:    "demo",
		AppType: "phx",
		Env:     map[string]string{"PHX_HOST": "user-value"},
	}, 50001)

	values := make(map[string]string)
	for _, item := range application.environment("demo.lab.s") {
		parts := strings.SplitN(item, "=", 2)
		values[parts[0]] = parts[1]
	}

	if values["PHX_HOST"] != "demo.lab.s" {
		t.Fatalf("PHX_HOST = %q, want demo.lab.s", values["PHX_HOST"])
	}
}
