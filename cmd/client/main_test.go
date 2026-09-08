package main

import (
	"testing"

	"github.com/cao7113/gater/entry/api"
)

func TestBuildNamesListIncludesPrimaryNamesAndAliases(t *testing.T) {
	apps := []api.AppInfo{
		{
			Name:         "livebook",
			Aliases:      []string{"lb", "lv"},
			DomainSuffix: ".l",
			URL:          "http://livebook.l",
		},
	}

	items := buildNamesList(apps)
	if len(items) != 3 {
		t.Fatalf("expected 3 name entries, got %d: %#v", len(items), items)
	}

	seen := map[string]bool{}
	for _, item := range items {
		seen[item.Value] = true
	}
	for _, value := range []string{"livebook", "lb", "lv"} {
		if !seen[value] {
			t.Fatalf("missing value %q in names list %#v", value, items)
		}
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
