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
		Cmd:     "mix",
		Args:    []string{"phx.server"},
		Env: []EnvKV{
			{Key: "PORT", Value: "${PORT}"},
			{Key: "PHX_HOST", Value: "${DOMAIN_HOST}"},
		},
		IdleTimeout: "10m",
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
		IdleTimeout: "15m",
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
