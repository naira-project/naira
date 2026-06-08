package main

import (
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/hashicorp/go-plugin"
	"github.com/naira-project/naira/catalog/internal/plugins/litellm"
	"github.com/naira-project/naira/catalog/pluginapi"
	"go-simpler.org/env"
)

type pluginConfig struct {
	Enabled     bool          `env:"LITELLM_ENABLED" default:"true"`
	BaseURL     string        `env:"LITELLM_BASE_URL" default:"http://127.0.0.1:4000"`
	APIKey      string        `env:"LITELLM_API_KEY"`
	HTTPTimeout time.Duration `env:"HTTP_TIMEOUT" default:"5s"`
}

func main() {
	var raw pluginConfig
	if err := env.Load(&raw, nil); err != nil {
		log.Fatalf("failed to load litellm config: %v", err)
	}

	logger := log.New(os.Stdout, "litellm-plugin ", log.LstdFlags)

	httpClient := &http.Client{
		Timeout: raw.HTTPTimeout,
	}

	impl := litellm.New(httpClient, logger, litellm.Config{
		Enabled: raw.Enabled,
		BaseURL: strings.TrimSpace(raw.BaseURL),
		APIKey:  strings.TrimSpace(raw.APIKey),
	})

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: pluginapi.HandshakeConfig,
		Plugins: map[string]plugin.Plugin{
			"catalog-plugin": &pluginapi.HashiPlugin{Impl: impl},
		},
		GRPCServer: plugin.DefaultGRPCServer,
	})
}
