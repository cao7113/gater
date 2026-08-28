package manager

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/cao7113/gater/internal/app"
	"github.com/cao7113/gater/internal/config"
	"github.com/cao7113/gater/internal/store"
)

var ErrAppExists = errors.New("应用已存在")

type Manager struct {
	mu       sync.RWMutex
	apps     map[string]*app.App
	store    *store.Store
	nextPort int
	ctx      context.Context
}

func New(ctx context.Context, st *store.Store) *Manager {
	m := &Manager{
		apps:     make(map[string]*app.App),
		store:    st,
		nextPort: 50001,
		ctx:      ctx,
	}

	// 从持久化存储恢复应用
	for _, ac := range st.List() {
		m.registerInstance(ac)
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

	if _, ok := m.apps[ac.Name]; ok {
		return fmt.Errorf("%w: [%s]，请使用修改接口", ErrAppExists, ac.Name)
	}

	if err := m.store.Save(ac); err != nil {
		return err
	}

	m.registerInstance(ac)
	log.Printf("[Gater] 成功注册应用: %s -> %s", ac.Name, ac.Cwd)
	return nil
}

func (m *Manager) UpdateApp(name string, cfg config.AppConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.apps[name]; !ok {
		return fmt.Errorf("应用 [%s] 不存在", name)
	}
	cfg.Name = name
	if err := config.Validate(cfg); err != nil {
		return fmt.Errorf("应用配置无效: %w", err)
	}
	m.apps[name].Stop()
	if err := m.store.Save(cfg); err != nil {
		return err
	}
	m.registerInstance(cfg)
	log.Printf("[Gater] 成功更新应用配置: %s", name)
	return nil
}

func (m *Manager) registerInstance(ac config.AppConfig) *app.App {
	port := m.nextPort
	m.nextPort++

	instance := app.NewApp(ac, port)
	m.apps[ac.Name] = instance

	go instance.MonitorIdle(m.ctx)
	return instance
}

func (m *Manager) GetApp(name string) (*app.App, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, ok := m.apps[name]
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

	if instance, ok := m.apps[name]; ok {
		instance.Stop()
		delete(m.apps, name)
	}

	return m.store.Delete(name)
}

func (m *Manager) StoreConfig() ([]byte, error) {
	return m.store.Content()
}
