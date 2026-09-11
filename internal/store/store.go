package store

import (
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/cao7113/gater/internal/config"
	"gopkg.in/yaml.v3"
)

type Store struct {
	mu        sync.Mutex
	filePath  string
	Apps      map[string]config.AppConfig `yaml:"apps" json:"apps"`
	AppsOrder []string                    `yaml:"apps_order,omitempty" json:"apps_order,omitempty"`
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

	if _, exists := s.Apps[ac.Name]; !exists {
		s.AppsOrder = append([]string{ac.Name}, s.AppsOrder...)
	}
	s.Apps[ac.Name] = ac
	return s.persist()
}

func (s *Store) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.Apps, name)
	for index, appName := range s.AppsOrder {
		if appName == name {
			s.AppsOrder = append(s.AppsOrder[:index], s.AppsOrder[index+1:]...)
			break
		}
	}
	return s.persist()
}

func (s *Store) Order() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]string(nil), s.AppsOrder...)
}

func (s *Store) SetOrder(order []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.AppsOrder = append([]string(nil), order...)
	return s.persist()
}

func (s *Store) Content() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return os.ReadFile(s.filePath)
}

func (s *Store) persist() error {
	data, err := yaml.Marshal(s)
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
	var document map[string]yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return err
	}
	if _, hasAppsKey := document["apps"]; hasAppsKey {
		if err := yaml.Unmarshal(data, s); err != nil {
			return err
		}
	} else {
		if err := yaml.Unmarshal(data, &s.Apps); err != nil {
			return err
		}
		s.AppsOrder = nil
	}
	if s.Apps == nil {
		s.Apps = make(map[string]config.AppConfig)
	}
	s.normalizeOrder()
	return nil
}

func (s *Store) normalizeOrder() {
	seen := make(map[string]struct{}, len(s.Apps))
	order := make([]string, 0, len(s.Apps))
	for _, name := range s.AppsOrder {
		if _, exists := s.Apps[name]; exists {
			if _, duplicate := seen[name]; !duplicate {
				order = append(order, name)
				seen[name] = struct{}{}
			}
		}
	}
	missing := make([]string, 0, len(s.Apps)-len(order))
	for name := range s.Apps {
		if _, exists := seen[name]; !exists {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	s.AppsOrder = append(order, missing...)
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
