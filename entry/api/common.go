package api

import (
	"time"

	"github.com/cao7113/gater/internal/app"
	"github.com/cao7113/gater/internal/config"
)

type appManager interface {
	GetAllApps() map[string]*app.App
	GetApp(name string) (*app.App, bool)
	AddOrUpdateApp(appYAMLPath string) error
	RegisterApp(cfg config.AppConfig) error
	UpdateApp(name string, cfg config.AppConfig) error
	RemoveApp(name string) error
	StoreConfig() ([]byte, error)
	AppSuffixes() []config.AppSuffix
	ServerConfig() ServerConfig
}

type AppInfo struct {
	Name             string            `json:"name"`
	DomainSuffix     string            `json:"domain_suffix"`
	URL              string            `json:"url"`
	AppType          string            `json:"app_type"`
	Cwd              string            `json:"cwd"`
	Cmd              string            `json:"cmd"`
	Args             []string          `json:"args"`
	Env              map[string]string `json:"env"`
	ConfigPort       int               `json:"config_port"`
	Port             int               `json:"port"`
	State            string            `json:"state"`
	IdleTimeoutSec   int               `json:"idle_timeout_sec"`
	RemainingSeconds int               `json:"remaining_seconds"`
	StartupMs        int64             `json:"startup_ms"`
	LastStartedAt    *time.Time        `json:"last_started_at"`
}

// ServerConfig 是 GET /api/config 返回的服务器运行时配置快照。
type ServerConfig struct {
	Version      string               `json:"version"`
	AdminPort    string               `json:"port"`
	AdminHost    string               `json:"admin_host"`
	TargetHost   string               `json:"target_host"`
	StorePath    string               `json:"store_path"`
	AppSuffixes  []config.AppSuffix   `json:"app_suffixes"`
	AppTemplates []config.AppTemplate `json:"app_templates"`
}
