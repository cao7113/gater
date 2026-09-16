package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cao7113/gater/internal/config"
)

func TestNewStoreRejectsInvalidExistingData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.yaml")
	if err := os.WriteFile(path, []byte("apps:\n  \"\":\n    name: \"\"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := NewStore(path); err == nil || !strings.Contains(err.Error(), "空应用名称") {
		t.Fatalf("NewStore error = %v, want invalid app error", err)
	}
}

func TestNewStoreRejectsAppConfigAsStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.yaml")
	if err := os.WriteFile(path, []byte("name: demo\ndomain_suffix: .l\ncmd: echo\nenv:\n  FOO: bar\n"), 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := NewStore(path); err == nil {
		t.Fatal("app config was accepted as a store")
	}
}

func TestSaveKeepsExistingStoreReadable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.yaml")
	st, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	appConfig := config.AppConfig{
		Name:         "demo",
		DomainSuffix: ".l",
		Cmd:          "echo",
		IdleTimeout:  "5m",
	}
	if err := st.Save(appConfig); err != nil {
		t.Fatal(err)
	}

	reloaded, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := reloaded.Get("demo"); !ok || got.Name != "demo" {
		t.Fatalf("saved app was not restored: %#v, %v", got, ok)
	}
}
