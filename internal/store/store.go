package store

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/cao7113/gater/internal/config"
	"gopkg.in/yaml.v3"
)

type Store struct {
	mu       sync.Mutex
	filePath string
	Apps     map[string]config.AppConfig `json:"apps"`
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
		Apps:     make(map[string]config.AppConfig),
	}
	_ = s.load()
	return s, nil
}

func (s *Store) List() map[string]config.AppConfig {
	s.mu.Lock()
	defer s.mu.Unlock()

	res := make(map[string]config.AppConfig)
	for k, v := range s.Apps {
		res[k] = v
	}
	return res
}

func (s *Store) Get(name string) (config.AppConfig, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ac, ok := s.Apps[name]
	return ac, ok
}

func (s *Store) Save(ac config.AppConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Apps[ac.Name] = ac
	return s.persist()
}

func (s *Store) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.Apps, name)
	return s.persist()
}

func (s *Store) Content() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return os.ReadFile(s.filePath)
}

func (s *Store) persist() error {
	data, err := yaml.Marshal(s.Apps)
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0644)
}

func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, &s.Apps)
}

func storePath(paths ...string) (string, error) {
	if len(paths) > 0 && paths[0] != "" {
		return paths[0], nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "gater", "store.yaml"), nil
}
