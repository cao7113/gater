package manager

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/cao7113/gater/internal/app"
	"github.com/cao7113/gater/internal/config"
	"github.com/cao7113/gater/internal/store"
)

var ErrAppExists = errors.New("应用已存在")

type Manager struct {
	mu              sync.RWMutex
	apps            map[string]*app.App
	store           *store.Store
	ctx             context.Context
	allowedSuffixes []config.AppSuffix
	names           map[string]string
	entries         map[string]entryRef
}

type entryRef struct {
	appName  string
	endpoint string
}

func New(ctx context.Context, st *store.Store, suffixes ...[]config.AppSuffix) *Manager {
	allowedSuffixes := config.DefaultSuffixes
	if len(suffixes) > 0 && len(suffixes[0]) > 0 {
		allowedSuffixes = suffixes[0]
	}
	m := &Manager{
		apps:            make(map[string]*app.App),
		store:           st,
		ctx:             ctx,
		allowedSuffixes: allowedSuffixes,
		names:           make(map[string]string),
		entries:         make(map[string]entryRef),
	}

	// 从持久化存储恢复应用
	for _, ac := range st.List() {
		if ac.DomainSuffix == "" {
			ac.DomainSuffix = allowedSuffixes[0].Suffix
			_ = st.Save(ac)
		}
		instance := m.registerInstance(ac)
		if err := m.addNames(ac); err != nil {
			log.Printf("[Gater] 应用名称或别名冲突，无法建立别名索引: %s: %v", ac.Name, err)
			instance.Config.Aliases = nil
		}
	}

	return m
}

func (m *Manager) AddOrUpdateApp(appYAMLPath string) error {
	loadedConfig, err := config.LoadFrom(appYAMLPath)
	if err != nil {
		return err
	}
	return m.RegisterApp(*loadedConfig)
}

func (m *Manager) RegisterApp(ac config.AppConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := config.Validate(ac); err != nil {
		return fmt.Errorf("应用配置无效: %w", err)
	}
	if err := config.ValidateDomainSuffix(ac.DomainSuffix, m.allowedSuffixes); err != nil {
		return fmt.Errorf("应用配置无效: %w", err)
	}

	if err := m.validateNames(ac, ""); err != nil {
		return err
	}

	if err := m.store.Save(ac); err != nil {
		return err
	}

	m.registerInstance(ac)
	if err := m.addNames(ac); err != nil {
		return err
	}
	log.Printf("[Gater] 注册应用成功: %s -> %s", ac.Name, ac.Cwd)
	return nil
}

func (m *Manager) UpdateApp(name string, cfg config.AppConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	canonical, ok := m.names[normalizeName(name)]
	if !ok {
		return fmt.Errorf("应用 [%s] 不存在", name)
	}
	cfg.Name = canonical
	if err := config.Validate(cfg); err != nil {
		return fmt.Errorf("应用配置无效: %w", err)
	}
	if err := config.ValidateDomainSuffix(cfg.DomainSuffix, m.allowedSuffixes); err != nil {
		return fmt.Errorf("应用配置无效: %w", err)
	}
	if err := m.validateNames(cfg, canonical); err != nil {
		return err
	}
	m.apps[canonical].Stop()
	if err := m.store.Save(cfg); err != nil {
		return err
	}
	m.removeNames(canonical)
	m.registerInstance(cfg)
	if err := m.addNames(cfg); err != nil {
		return err
	}
	log.Printf("[Gater] 成功更新应用配置: %s", canonical)
	return nil
}

func (m *Manager) GetEntry(name string) (*app.App, string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.entries[normalizeName(name)]
	if !ok {
		return nil, "", false
	}
	a, ok := m.apps[entry.appName]
	if !ok {
		return nil, "", false
	}
	return a, entry.endpoint, true
}

func (m *Manager) registerInstance(ac config.AppConfig) *app.App {
	instance := app.NewApp(ac)
	m.apps[ac.Name] = instance

	go instance.MonitorIdle(m.ctx)
	return instance
}

func (m *Manager) GetApp(name string) (*app.App, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	canonical, ok := m.names[normalizeName(name)]
	if !ok {
		return nil, false
	}
	a, ok := m.apps[canonical]
	return a, ok
}

func (m *Manager) GetAllApps() map[string]*app.App {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make(map[string]*app.App)
	for k, v := range m.apps {
		res[k] = v
	}
	return res
}

func (m *Manager) RemoveApp(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	canonical, ok := m.names[normalizeName(name)]
	if !ok {
		return nil
	}
	if instance, ok := m.apps[canonical]; ok {
		instance.Stop()
		delete(m.apps, canonical)
	}
	m.removeNames(canonical)

	return m.store.Delete(canonical)
}

func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func (m *Manager) validateNames(ac config.AppConfig, current string) error {
	seen := make(map[string]struct{}, len(ac.Aliases)+len(ac.Endpoints)+1)
	check := func(value, kind string) error {
		name := normalizeName(value)
		if name == "" {
			return fmt.Errorf("应用配置无效: %s 不能为空", kind)
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("应用入口名称重复: [%s]", value)
		}
		seen[name] = struct{}{}
		if owner, exists := m.names[name]; exists && owner != current {
			return fmt.Errorf("%w: [%s]，名称或别名已被应用 [%s] 使用", ErrAppExists, value, owner)
		}
		return nil
	}
	if err := check(ac.Name, "name"); err != nil {
		return err
	}
	for _, alias := range ac.Aliases {
		if err := check(alias, "alias"); err != nil {
			return err
		}
	}
	for _, endpoint := range ac.Endpoints {
		if err := check(endpoint.EntryName, "endpoint entry_name"); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) addNames(ac config.AppConfig) error {
	if err := m.validateNames(ac, ""); err != nil {
		return err
	}
	canonical := strings.TrimSpace(ac.Name)
	m.names[normalizeName(canonical)] = canonical
	m.entries[normalizeName(canonical)] = entryRef{appName: canonical}
	for _, alias := range ac.Aliases {
		name := normalizeName(alias)
		m.names[name] = canonical
		m.entries[name] = entryRef{appName: canonical}
	}
	for _, endpoint := range ac.Endpoints {
		name := normalizeName(endpoint.EntryName)
		m.names[name] = canonical
		m.entries[name] = entryRef{appName: canonical, endpoint: name}
	}
	return nil
}

func (m *Manager) removeNames(canonical string) {
	for name, owner := range m.names {
		if owner == canonical {
			delete(m.names, name)
			delete(m.entries, name)
		}
	}
}

func (m *Manager) StoreConfig() ([]byte, error) {
	return m.store.Content()
}
