package main

import (
	"testing"

	"github.com/cao7113/gater/entry/api"
	"github.com/cao7113/gater/internal/config"
)

func TestBuildNamesListIncludesPrimaryNamesAndAliases(t *testing.T) {
	apps := []api.AppInfo{
		{
			Name:         "livebook",
			Aliases:      []string{"lb", "lv"},
			Endpoints:    []config.EndpointConfig{{EntryName: "livebook-iframe", Label: "iframe", PortEnv: "IFRAME_PORT"}},
			DomainSuffix: ".l",
			URL:          "http://livebook.l",
		},
	}

	items := buildNamesList(apps)
	if len(items) != 4 {
		t.Fatalf("expected 4 name entries, got %d: %#v", len(items), items)
	}

	seen := map[string]bool{}
	for _, item := range items {
		seen[item.Value] = true
	}
	for _, value := range []string{"livebook", "lb", "lv", "livebook-iframe"} {
		if !seen[value] {
			t.Fatalf("missing value %q in names list %#v", value, items)
		}
	}
}

func TestEndpointURL(t *testing.T) {
	app := api.AppInfo{DomainSuffix: ".s", URL: "https://livebook.s"}
	if got := endpointURL(app, "livebook-iframe"); got != "https://livebook-iframe.s" {
		t.Fatalf("endpointURL() = %q", got)
	}
}

func TestBuildNamesListFiltersByKeyword(t *testing.T) {
	apps := []api.AppInfo{
		{Name: "demo", Aliases: []string{"d"}},
		{Name: "livebook", Aliases: []string{"lb", "lv"}},
	}

	filtered := filterNames(buildNamesList(apps), "lb")
	if len(filtered) != 1 || filtered[0].Value != "lb" {
		t.Fatalf("expected lb alias entry, got %#v", filtered)
	}
}
