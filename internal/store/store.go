package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type AppSpec struct {
	Name        string            `json:"name"`
	Path        string            `json:"path"`
	Cmd         string            `json:"cmd"`
	Args        []string          `json:"args"`
	Env         map[string]string `json:"env"`
	IdleTimeout string            `json:"idle_timeout"` // e.g. "5m", "1h"
}

type Store struct {
	mu       sync.Mutex
	filePath string
	Apps     map[string]AppSpec `json:"apps"`
}

func NewStore(paths ...string) (*Store, error) {
	filePath, err := storePath(paths...)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return nil, err
	}

	s := &Store{
		filePath: filePath,
		Apps:     make(map[string]AppSpec),
	}
	_ = s.load()
	return s, nil
}

func storePath(paths ...string) (string, error) {
	if len(paths) > 0 && paths[0] != "" {
		return paths[0], nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "gater", "store.json"), nil
}

func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &s.Apps)
}

func (s *Store) Save(spec AppSpec) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Apps[spec.Name] = spec
	return s.persist()
}

func (s *Store) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.Apps, name)
	return s.persist()
}

func (s *Store) Get(name string) (AppSpec, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	spec, ok := s.Apps[name]
	return spec, ok
}

func (s *Store) List() map[string]AppSpec {
	s.mu.Lock()
	defer s.mu.Unlock()

	res := make(map[string]AppSpec)
	for k, v := range s.Apps {
		res[k] = v
	}
	return res
}

func (s *Store) persist() error {
	data, err := json.MarshalIndent(s.Apps, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0644)
}
