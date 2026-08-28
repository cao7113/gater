package app

import (
	"strings"
	"testing"

	"github.com/cao7113/gater/internal/app/types"
	"github.com/cao7113/gater/internal/config"
)

func TestEnvironmentInjectsPortAndAppDomain(t *testing.T) {
	application := NewApp(config.AppConfig{
		Name:         "demo",
		DomainSuffix: ".l.s",
		Env:          map[string]string{"APP_DOMAIN": "user-value", "FOO": "bar"},
	}, 50001)

	values := make(map[string]string)
	if application.Domain() != "demo.l.s" {
		t.Fatalf("Domain() = %q, want demo.l.s", application.Domain())
	}
	appTypeContext := application.newAppTypeContext()
	if err := types.HandlerFor(application.Config.AppType).Prepare(appTypeContext); err != nil {
		t.Fatal(err)
	}
	for _, item := range envList(appTypeContext.Env) {
		parts := strings.SplitN(item, "=", 2)
		values[parts[0]] = parts[1]
	}

	if values["PORT"] != "50001" {
		t.Fatalf("PORT = %q, want 50001", values["PORT"])
	}
	if values["APP_DOMAIN"] != "demo.l.s" {
		t.Fatalf("APP_DOMAIN = %q, want demo.l.s", values["APP_DOMAIN"])
	}
	if values["FOO"] != "bar" {
		t.Fatalf("FOO = %q, want bar", values["FOO"])
	}
}

func TestEnvironmentInjectsPhxHost(t *testing.T) {
	application := NewApp(config.AppConfig{
		Name:         "demo",
		DomainSuffix: ".l.s",
		AppType:      config.AppTypePhoenix,
		Env:          map[string]string{"PHX_HOST": "user-value"},
	}, 50001)

	values := make(map[string]string)
	appTypeContext := application.newAppTypeContext()
	if err := types.HandlerFor(application.Config.AppType).Prepare(appTypeContext); err != nil {
		t.Fatal(err)
	}
	for _, item := range envList(appTypeContext.Env) {
		parts := strings.SplitN(item, "=", 2)
		values[parts[0]] = parts[1]
	}

	if values["PHX_HOST"] != "demo.l.s" {
		t.Fatalf("PHX_HOST = %q, want demo.l.s", values["PHX_HOST"])
	}
}

func TestURLUsesAppDomain(t *testing.T) {
	application := NewApp(config.AppConfig{Name: "demo", DomainSuffix: ".l.h"}, 50001)
	if got := application.URL(); got != "http://demo.l.h" {
		t.Fatalf("URL() = %q, want http://demo.l.h", got)
	}
	if got := application.URL("https"); got != "https://demo.l.h" {
		t.Fatalf("URL(https) = %q, want https://demo.l.h", got)
	}
}

func TestAppTypeHandlerFor(t *testing.T) {
	appTypeContext := &types.TypeContext{
		Config: config.AppConfig{AppType: config.AppTypePhoenix},
		Domain: "demo.l.h",
		Env:    map[string]string{},
	}
	if err := types.HandlerFor(config.AppTypePhoenix).Prepare(appTypeContext); err != nil {
		t.Fatal(err)
	}
	if appTypeContext.Env["PHX_HOST"] != "demo.l.h" {
		t.Fatalf("PHX_HOST = %q, want demo.l.h", appTypeContext.Env["PHX_HOST"])
	}

	defaultContext := &types.TypeContext{Env: map[string]string{}}
	if err := types.HandlerFor("unknown").Prepare(defaultContext); err != nil {
		t.Fatal(err)
	}
	if _, ok := defaultContext.Env["PHX_HOST"]; ok {
		t.Fatal("default handler unexpectedly added PHX_HOST")
	}
}
