package config

const AppTypePhoenix = "phx"

type AppTemplate struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	AppType     string   `json:"app_type"`
	Cmd         string   `json:"cmd"`
	Args        []string `json:"args"`
	IdleTimeout string   `json:"idle_timeout"`
}

var DefaultAppTemplates = []AppTemplate{
	{ID: AppTypePhoenix, Label: "Phoenix（Elixir）", AppType: AppTypePhoenix, Cmd: "mix", Args: []string{"phx.server"}, IdleTimeout: "10m"},
	{ID: "bun", Label: "Bun（Dev）", AppType: "bun", Cmd: "bun", Args: []string{"run", "dev", "--port", "$PORT"}, IdleTimeout: "15m"},
	{ID: "python", Label: "Python HTTP", AppType: "python", Cmd: "python3", Args: []string{"-m", "http.server", "$PORT"}, IdleTimeout: "5m"},
}
