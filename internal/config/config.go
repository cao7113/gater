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

const DefaultIdleTimeout = "10m"

type AppConfig struct {
	Name         string            `yaml:"name" json:"name"`
	DomainSuffix string            `yaml:"domain_suffix" json:"domain_suffix"`
	AppType      string            `yaml:"app_type" json:"app_type"`
	Cwd          string            `yaml:"cwd" json:"cwd"`
	Cmd          string            `yaml:"cmd" json:"cmd"`
	Args         []string          `yaml:"args" json:"args"`
	Env          map[string]string `yaml:"env" json:"env"`
	IdleTimeout  string            `yaml:"idle_timeout" json:"idle_timeout"`
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
	cfg.Cwd = appDir

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
	if strings.TrimSpace(cfg.Cwd) == "" {
		return fmt.Errorf("缺少 cwd")
	}
	if cfg.Cmd == "" {
		return fmt.Errorf("缺少 cmd")
	}
	if strings.TrimSpace(cfg.DomainSuffix) == "" {
		return fmt.Errorf("缺少 domain_suffix")
	}

	duration, err := ParseTimeout(cfg.IdleTimeout)
	if err != nil || duration <= 0 {
		return fmt.Errorf("idle_timeout 无效: %q", cfg.IdleTimeout)
	}

	return nil
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
	return fmt.Errorf("domain_suffix 不被允许: %q", suffix)
}

func ParseTimeout(du string) (time.Duration, error) {
	return time.ParseDuration(du)
}
