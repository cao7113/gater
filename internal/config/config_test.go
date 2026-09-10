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
