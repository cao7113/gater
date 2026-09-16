package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	DefaultIdleTimeout = "5m"
	DefaultTargetHost  = "127.0.0.1"
)

var TargetHost = DefaultTargetHost

type AppConfig struct {
	Name         string                   `yaml:"name" json:"name"`
	Aliases      []string                 `yaml:"aliases,omitempty" json:"aliases,omitempty"`
	Endpoints    []EndpointConfig         `yaml:"endpoints,omitempty" json:"endpoints,omitempty"`
	DomainSuffix string                   `yaml:"domain_suffix" json:"domain_suffix"`
	AppType      string                   `yaml:"app_type" json:"app_type"`
	Cwd          string                   `yaml:"cwd" json:"cwd"`
	Cmd          string                   `yaml:"cmd" json:"cmd"`
	Args         []string                 `yaml:"args" json:"args"`
	Env          map[string]string        `yaml:"env" json:"env"`
	Commands     map[string]CommandConfig `yaml:"commands,omitempty" json:"commands,omitempty"`
	Port         int                      `yaml:"port,omitempty" json:"port,omitempty"`
	IdleTimeout  string                   `yaml:"idle_timeout" json:"idle_timeout"`
}

type CommandConfig struct {
	Cmd      string            `yaml:"cmd,omitempty" json:"cmd,omitempty"`
	Args     []string          `yaml:"args,omitempty" json:"args,omitempty"`
	Cwd      string            `yaml:"cwd,omitempty" json:"cwd,omitempty"`
	Env      map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	UnsetEnv []string          `yaml:"unset_env,omitempty" json:"unset_env,omitempty"`
}

type EndpointConfig struct {
	EntryName string `yaml:"entry_name" json:"entry_name"`
	Label     string `yaml:"label,omitempty" json:"label,omitempty"`
	PortEnv   string `yaml:"port_env" json:"port_env"`
}

func (e EndpointConfig) DisplayLabel() string {
	if strings.TrimSpace(e.Label) != "" {
		return e.Label
	}
	return e.EntryName
}

func LoadFrom(yamlPath string) (*AppConfig, error) {
	yamlPath, err := ResolvePath(yamlPath)
	if err != nil {
		return nil, fmt.Errorf("无法解析 app.yaml 路径: %w", err)
	}

	stat, err := os.Stat(yamlPath)
	if err != nil {
		return nil, fmt.Errorf("app.yaml 文件无效: %w", err)
	}
	if stat.IsDir() {
		return nil, fmt.Errorf("app.yaml 路径是目录: %s", yamlPath)
	}

	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return nil, fmt.Errorf("读取 app.yaml 失败: %w", err)
	}

	var cfg AppConfig
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("解析 app.yaml 失败: %w", err)
	}

	appDir, err := filepath.Abs(filepath.Dir(yamlPath))
	if err != nil {
		return nil, fmt.Errorf("解析应用目录失败: %w", err)
	}
	if strings.TrimSpace(cfg.Cwd) == "" {
		cfg.Cwd = appDir
	}

	if cfg.IdleTimeout == "" {
		cfg.IdleTimeout = DefaultIdleTimeout
	}

	if err := Validate(cfg); err != nil {
		return nil, fmt.Errorf("app.yaml 配置无效: %w", err)
	}

	return &cfg, nil
}

func ResolvePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, path[2:])
		}
	}
	return filepath.Abs(path)
}

func Validate(cfg AppConfig) error {
	if cfg.Name == "" {
		return fmt.Errorf("缺少 name")
	}
	if cfg.Cmd == "" {
		return fmt.Errorf("缺少 cmd")
	}
	if strings.TrimSpace(cfg.DomainSuffix) == "" {
		return fmt.Errorf("缺少 domain_suffix")
	}
	if cfg.Port < 0 || cfg.Port > 65535 {
		return fmt.Errorf("port 无效: %d", cfg.Port)
	}
	for name, command := range cfg.Commands {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("command 名称不能为空")
		}
		if strings.TrimSpace(command.Cmd) == "" && len(command.Args) == 0 && strings.TrimSpace(command.Cwd) == "" && len(command.Env) == 0 && len(command.UnsetEnv) == 0 {
			return fmt.Errorf("command [%s] 不能为空", name)
		}
		for envName := range command.Env {
			if !isEnvName(envName) {
				return fmt.Errorf("command [%s] 环境变量名无效: %q", name, envName)
			}
		}
		unsetNames := make(map[string]struct{}, len(command.UnsetEnv))
		for _, envName := range command.UnsetEnv {
			envName = strings.TrimSpace(envName)
			if !isEnvName(envName) {
				return fmt.Errorf("command [%s] unset_env 变量名无效: %q", name, envName)
			}
			if _, exists := unsetNames[envName]; exists {
				return fmt.Errorf("command [%s] unset_env 变量重复: %q", name, envName)
			}
			if _, exists := command.Env[envName]; exists {
				return fmt.Errorf("command [%s] 不能同时设置和删除环境变量: %q", name, envName)
			}
			unsetNames[envName] = struct{}{}
		}
	}
	seenEndpoints := make(map[string]struct{}, len(cfg.Endpoints))
	seenPortEnvs := make(map[string]struct{}, len(cfg.Endpoints))
	for _, endpoint := range cfg.Endpoints {
		entryName := strings.ToLower(strings.TrimSpace(endpoint.EntryName))
		if entryName == "" {
			return fmt.Errorf("endpoint 缺少 entry_name")
		}
		if !isDNSLabel(entryName) {
			return fmt.Errorf("endpoint entry_name 无效: %q", endpoint.EntryName)
		}
		if entryName == "main" {
			return fmt.Errorf("endpoint entry_name 不能是 main")
		}
		if _, exists := seenEndpoints[entryName]; exists {
			return fmt.Errorf("endpoint entry_name 重复: %q", endpoint.EntryName)
		}
		seenEndpoints[entryName] = struct{}{}
		portEnv := strings.TrimSpace(endpoint.PortEnv)
		if !isEnvName(portEnv) {
			return fmt.Errorf("endpoint port_env 无效: %q", endpoint.PortEnv)
		}
		if portEnv == "PORT" {
			return fmt.Errorf("endpoint port_env 不能使用 PORT")
		}
		if _, exists := seenPortEnvs[portEnv]; exists {
			return fmt.Errorf("endpoint port_env 重复: %q", endpoint.PortEnv)
		}
		seenPortEnvs[portEnv] = struct{}{}
	}

	duration, err := ParseTimeout(cfg.IdleTimeout)
	if err != nil || duration <= 0 {
		return fmt.Errorf("idle_timeout 无效: %q", cfg.IdleTimeout)
	}

	return nil
}

func isDNSLabel(value string) bool {
	if len(value) == 0 || len(value) > 63 || value[0] == '-' || value[len(value)-1] == '-' {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
			return false
		}
	}
	return true
}

func isEnvName(value string) bool {
	if value == "" {
		return false
	}
	for index, char := range value {
		if (char < 'A' || char > 'Z') && (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '_' {
			return false
		}
		if index == 0 && char >= '0' && char <= '9' {
			return false
		}
	}
	return true
}

func ValidateDomainSuffix(suffix string, allowed []AppSuffix) error {
	suffix = strings.TrimSpace(suffix)
	if suffix == "" {
		return fmt.Errorf("缺少 domain_suffix")
	}
	for _, item := range allowed {
		if suffix == item.Suffix {
			return nil
		}
	}
	return fmt.Errorf("domain_suffix 不被允许: %q, allowed suffixes: %v", suffix, allowed)
}

func ParseTimeout(du string) (time.Duration, error) {
	return time.ParseDuration(du)
}
