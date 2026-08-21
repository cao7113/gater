package manager

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/cao7113/gater/internal/app"
	"github.com/cao7113/gater/internal/config"
	"github.com/cao7113/gater/internal/store"
)

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
	for _, spec := range st.List() {
		m.registerInstance(spec)
	}

	return m
}

func (m *Manager) registerInstance(spec store.AppSpec) *app.App {
	port := m.nextPort
	m.nextPort++

	instance := app.NewApp(spec, port)
	m.apps[spec.Name] = instance

	go instance.MonitorIdle(m.ctx)
	return instance
}

// ExpandHomePath 展开包含 ~ 的用户主目录路径
func ExpandHomePath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "~" {
		return os.UserHomeDir()
	}
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, p[2:]), nil
	}
	return p, nil
}

func (m *Manager) AddOrUpdateApp(spec store.AppSpec) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	expandedPath, err := ExpandHomePath(spec.Path)
	if err != nil {
		return fmt.Errorf("无法解析用户目录路径: %w", err)
	}

	absPath, err := filepath.Abs(expandedPath)
	if err != nil {
		return fmt.Errorf("无效路径: %w", err)
	}
	if fi, err := os.Stat(absPath); err != nil || !fi.IsDir() {
		return fmt.Errorf("工作目录不存在: %s", absPath)
	}
	spec.Path = absPath

	// 如果应用目录包含 app.yaml，尝试读取默认配置做补充
	if cfg, _, err := config.LoadAppConfig(absPath); err == nil {
		if spec.Cmd == "" {
			spec.Cmd = cfg.Cmd
		}
		if len(spec.Args) == 0 {
			spec.Args = cfg.Args
		}
	}

	if spec.Cmd == "" {
		return fmt.Errorf("缺少启动命令 (Cmd)")
	}

	if existing, ok := m.apps[spec.Name]; ok {
		existing.Stop()
	}

	if err := m.store.Save(spec); err != nil {
		return err
	}

	m.registerInstance(spec)
	log.Printf("[Gater] 成功注册应用: %s.lab -> %s", spec.Name, spec.Path)
	return nil
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
