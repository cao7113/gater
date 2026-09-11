package api

import (
	"time"

	"github.com/cao7113/gater/internal/app"
	"github.com/cao7113/gater/internal/config"
)

type appManager interface {
	GetAllApps() map[string]*app.App
	GetAppsOrder() []string
	GetApp(name string) (*app.App, bool)
	AddOrUpdateApp(appYAMLPath string) error
	RegisterApp(cfg config.AppConfig) error
	UpdateApp(name string, cfg config.AppConfig) error
	RemoveApp(name string) error
	SetAppsOrder(order []string) error
	StoreConfig() ([]byte, error)
	AppSuffixes() []config.AppSuffix
	ServerConfig() ServerConfig
}

type AppInfo struct {
	Name             string                  `json:"name"`
	Aliases          []string                `json:"aliases,omitempty"`
	Endpoints        []config.EndpointConfig `json:"endpoints,omitempty"`
	EndpointPorts    map[string]int          `json:"endpoint_ports,omitempty"`
	DomainSuffix     string                  `json:"domain_suffix"`
	URL              string                  `json:"url"`
	AppType          string                  `json:"app_type"`
	Cwd              string                  `json:"cwd"`
	Cmd              string                  `json:"cmd"`
	Args             []string                `json:"args"`
	Env              map[string]string       `json:"env"`
	ConfigPort       int                     `json:"config_port"`
	Port             int                     `json:"port"`
	State            string                  `json:"state"`
	IdleTimeoutSec   int                     `json:"idle_timeout_sec"`
	RemainingSeconds int                     `json:"remaining_seconds"`
	StartupMs        int64                   `json:"startup_ms"`
	LastStartedAt    *time.Time              `json:"last_started_at"`
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

type RuntimeInfo struct {
	PID         int                 `json:"pid"`
	PPID        int                 `json:"ppid"`
	User        string              `json:"user,omitempty"`
	UID         string              `json:"uid,omitempty"`
	GID         string              `json:"gid,omitempty"`
	Executable  string              `json:"executable,omitempty"`
	Args        []string            `json:"args"`
	CWD         string              `json:"cwd,omitempty"`
	Runtime     RuntimeEnvironment  `json:"runtime"`
	Environment map[string]EnvValue `json:"environment"`
}

type RuntimeEnvironment struct {
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	CPUs      int    `json:"cpus"`
}

type EnvValue struct {
	Set      bool   `json:"set"`
	Value    string `json:"value,omitempty"`
	Redacted bool   `json:"redacted,omitempty"`
}
