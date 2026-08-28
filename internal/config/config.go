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

// AppSuffix 定义应用域名后缀及对应的访问协议。
type AppSuffix struct {
	Suffix string `yaml:"suffix" json:"suffix"`
	Scheme string `yaml:"scheme" json:"scheme"`
}

type AppTemplate struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	AppType     string   `json:"app_type"`
	Cmd         string   `json:"cmd"`
	Args        []string `json:"args"`
	IdleTimeout string   `json:"idle_timeout"`
}

var DefaultAppTemplates = []AppTemplate{
	{ID: "phx", Label: "Phoenix（Elixir）", AppType: "phx", Cmd: "mix", Args: []string{"phx.server"}, IdleTimeout: "10m"},
	{ID: "bun", Label: "Bun（Dev）", AppType: "bun", Cmd: "bun", Args: []string{"run", "dev", "--port", "$PORT"}, IdleTimeout: "15m"},
	{ID: "python", Label: "Python HTTP", AppType: "python", Cmd: "python3", Args: []string{"-m", "http.server", "$PORT"}, IdleTimeout: "5m"},
}

// DefaultSuffixes 是默认的应用域名后缀及协议列表。
var DefaultSuffixes = []AppSuffix{
	{Suffix: ".lab.s", Scheme: "https"},
	{Suffix: ".lab", Scheme: "http"},
	{Suffix: ".l.h", Scheme: "http"},
}

// ParseAppSuffix 从字符串解析 AppSuffix（支持 "https:.lab.s", ".lab.s:https", ".lab.s" 等格式）。
func ParseAppSuffix(raw string) AppSuffix {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "https:") || strings.HasPrefix(raw, "https://") {
		s := strings.TrimPrefix(strings.TrimPrefix(raw, "https://"), "https:")
		return AppSuffix{Suffix: s, Scheme: "https"}
	}
	if strings.HasPrefix(raw, "http:") || strings.HasPrefix(raw, "http://") {
		s := strings.TrimPrefix(strings.TrimPrefix(raw, "http://"), "http:")
		return AppSuffix{Suffix: s, Scheme: "http"}
	}
	if parts := strings.Split(raw, ":"); len(parts) == 2 {
		if parts[1] == "https" || parts[1] == "http" {
			return AppSuffix{Suffix: parts[0], Scheme: parts[1]}
		}
		if parts[0] == "https" || parts[0] == "http" {
			return AppSuffix{Suffix: parts[1], Scheme: parts[0]}
		}
	}
	scheme := "http"
	if strings.HasSuffix(raw, ".s") {
		scheme = "https"
	}
	return AppSuffix{Suffix: raw, Scheme: scheme}
}

type AppConfig struct {
	Name        string            `yaml:"name" json:"name"`
	AppType     string            `yaml:"app_type" json:"app_type"`
	Cwd         string            `yaml:"cwd" json:"cwd"`
	Cmd         string            `yaml:"cmd" json:"cmd"`
	Args        []string          `yaml:"args" json:"args"`
	Env         map[string]string `yaml:"env" json:"env"`
	IdleTimeout string            `yaml:"idle_timeout" json:"idle_timeout"`
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

	duration, err := ParseTimeout(cfg.IdleTimeout)
	if err != nil || duration <= 0 {
		return fmt.Errorf("idle_timeout 无效: %q", cfg.IdleTimeout)
	}

	return nil
}

func ParseTimeout(du string) (time.Duration, error) {
	return time.ParseDuration(du)
}
