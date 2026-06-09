package main

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/go-plugin"
	"github.com/naira-project/naira/catalog/pluginapi"
	"go-simpler.org/env"
)

type pluginConfig struct {
	Enabled     bool          `env:"MLFLOW_ENABLED" default:"true"`
	BaseURL     string        `env:"MLFLOW_BASE_URL" default:"http://127.0.0.1:5000"`
	BearerToken string        `env:"MLFLOW_BEARER_TOKEN"`
	HTTPTimeout time.Duration `env:"HTTP_TIMEOUT" default:"5s"`
}

func main() {
	var raw pluginConfig
	if err := env.Load(&raw, nil); err != nil {
		log.Fatalf("failed to load mlflow config: %v", err)
	}

	httpClient := &http.Client{
		Timeout: raw.HTTPTimeout,
	}

	impl := New(httpClient, Config{
		Enabled:     raw.Enabled,
		BaseURL:     strings.TrimSpace(raw.BaseURL),
		BearerToken: strings.TrimSpace(raw.BearerToken),
	})

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: pluginapi.HandshakeConfig,
		Plugins: map[string]plugin.Plugin{
			"catalog-plugin": &pluginapi.HashiPlugin{Impl: impl},
		},
		GRPCServer: plugin.DefaultGRPCServer,
	})
}
