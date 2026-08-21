package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type AppConfig struct {
	Name        string            `yaml:"name"`
	Cmd         string            `yaml:"cmd"`
	Args        []string          `yaml:"args"`
	Env         map[string]string `yaml:"env"`
	IdleTimeout string            `yaml:"idle_timeout"`
}

func LoadAppConfig(appDir string) (*AppConfig, time.Duration, error) {
	yamlPath := filepath.Join(appDir, "app.yaml")
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return nil, 0, fmt.Errorf("读取 app.yaml 失败: %w", err)
	}

	var cfg AppConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, 0, fmt.Errorf("解析 app.yaml 失败: %w", err)
	}

	if cfg.Name == "" {
		cfg.Name = filepath.Base(appDir)
	}

	timeout := 5 * time.Minute
	if cfg.IdleTimeout != "" {
		if d, err := time.ParseDuration(cfg.IdleTimeout); err == nil {
			timeout = d
		}
	}

	return &cfg, timeout, nil
}

// ExpandEnv 展开参数或环境变量中的 $PORT 及系统环境变量
func ExpandEnv(input string, port int) string {
	return os.Expand(input, func(key string) string {
		if key == "PORT" {
			return fmt.Sprintf("%d", port)
		}
		return os.Getenv(key)
	})
}