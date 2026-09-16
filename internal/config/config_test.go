package config

import "testing"

func TestValidateEndpoints(t *testing.T) {
	base := AppConfig{
		Name:         "livebook",
		DomainSuffix: ".s",
		Cmd:          "livebook",
		IdleTimeout:  "5m",
	}

	valid := base
	valid.Endpoints = []EndpointConfig{{EntryName: "livebook-iframe", Label: "iframe", PortEnv: "IFRAME_PORT"}}
	if err := Validate(valid); err != nil {
		t.Fatalf("valid endpoint config rejected: %v", err)
	}

	tests := []struct {
		name      string
		endpoints []EndpointConfig
	}{
		{"missing entry name", []EndpointConfig{{PortEnv: "IFRAME_PORT"}}},
		{"invalid entry name", []EndpointConfig{{EntryName: "livebook.iframe", PortEnv: "IFRAME_PORT"}}},
		{"missing port env", []EndpointConfig{{EntryName: "iframe"}}},
		{"invalid port env", []EndpointConfig{{EntryName: "iframe", PortEnv: "1_PORT"}}},
		{"main port env", []EndpointConfig{{EntryName: "iframe", PortEnv: "PORT"}}},
		{"duplicate entry name", []EndpointConfig{
			{EntryName: "iframe", PortEnv: "IFRAME_PORT"},
			{EntryName: "IFRAME", PortEnv: "OTHER_PORT"},
		}},
		{"duplicate port env", []EndpointConfig{
			{EntryName: "iframe", PortEnv: "PORT"},
			{EntryName: "assets", PortEnv: "PORT"},
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := base
			cfg.Endpoints = test.endpoints
			if err := Validate(cfg); err == nil {
				t.Fatal("invalid endpoint config was accepted")
			}
		})
	}
}

func TestValidateCommands(t *testing.T) {
	base := AppConfig{Name: "demo", DomainSuffix: ".l", Cmd: "mix", IdleTimeout: "5m"}
	valid := base
	valid.Commands = map[string]CommandConfig{"migrate": {Args: []string{"ecto.migrate"}}}
	if err := Validate(valid); err != nil {
		t.Fatalf("valid command config rejected: %v", err)
	}

	invalid := base
	invalid.Commands = map[string]CommandConfig{"": {Cmd: "mix"}}
	if err := Validate(invalid); err == nil {
		t.Fatal("empty command name was accepted")
	}

	validUnset := base
	validUnset.Commands = map[string]CommandConfig{"migrate": {UnsetEnv: []string{"PHX_SERVER"}}}
	if err := Validate(validUnset); err != nil {
		t.Fatalf("valid unset_env rejected: %v", err)
	}

	invalidUnset := base
	invalidUnset.Commands = map[string]CommandConfig{"migrate": {Env: map[string]string{"PHX_SERVER": "1"}, UnsetEnv: []string{"PHX_SERVER"}}}
	if err := Validate(invalidUnset); err == nil {
		t.Fatal("command that sets and unsets the same variable was accepted")
	}
}
