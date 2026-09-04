package config

const AppTypePhx = "phx"

type EnvKV struct {
	Key   string `json:"key"`
	Value string `json:"value"` // 修复：补全 JSON Tag 的闭合双引号
}

type AppTemplate struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	AppType     string   `json:"app_type"`
	Cmd         string   `json:"cmd"`
	Args        []string `json:"args"`
	Env         []EnvKV  `json:"env"`
	IdleTimeout string   `json:"idle_timeout"`
}

var DefaultAppTemplates = []AppTemplate{
	{
		ID:      AppTypePhx,
		Label:   "Phoenix（Elixir）",
		AppType: AppTypePhx,
		Cmd:     "/path/to/_build/prod/rel/xxx/bin/xxx",
		Args:    []string{"start"},
		Env: []EnvKV{
			{Key: "PORT", Value: "${PORT}"},
			{Key: "PHX_HOST", Value: "${APP_DOMAIN}"},
			{Key: "PHX_SERVER", Value: "1"},
			{Key: "SECRET_KEY_BASE", Value: "todo mix phx.gen.secret"},
			{Key: "DATABASE_URL", Value: "postgres://postgres:postgres@localhost:5432/xxx_prod?sslmode=disable"},
		},
		IdleTimeout: "5m",
	},
	{
		ID:      "fs",
		Label:   "File Server",
		AppType: "fs",
		// Cwd:     "/path/to/gater",
		Cmd:  "caddy",
		Args: []string{"file-server", "--browse", "--listen", ":${PORT}", "--root", "."},
		Env: []EnvKV{
			{Key: "PORT", Value: "${PORT}"},
		},
		IdleTimeout: "5m",
	},
	{
		ID:      "bun",
		Label:   "Bun（Dev）",
		AppType: "bun",
		Cmd:     "bun",
		Args:    []string{"run", "dev", "--port", "${PORT}"},
		Env: []EnvKV{
			{Key: "PORT", Value: "${PORT}"},
		},
		IdleTimeout: "5m",
	},
	{
		ID:      "python",
		Label:   "Python HTTP",
		AppType: "python",
		Cmd:     "python3",
		Args:    []string{"-m", "http.server", "${PORT}"},
		Env: []EnvKV{
			{Key: "PORT", Value: "${PORT}"},
		},
		IdleTimeout: "5m",
	},
}
