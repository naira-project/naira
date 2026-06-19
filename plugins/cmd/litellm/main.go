package main

import (
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/naira-project/naira/plugins/internal/pluginmain"
	"go-simpler.org/env"
)

const defaultPort = 50051

type pluginConfig struct {
	Enabled     bool          `env:"LITELLM_ENABLED" default:"true"`
	BaseURL     string        `env:"LITELLM_BASE_URL" default:"http://127.0.0.1:4000"`
	APIKey      string        `env:"LITELLM_API_KEY"`
	HTTPTimeout time.Duration `env:"LITELLM_HTTP_TIMEOUT" default:"5s"`
}

func main() {
	var raw pluginConfig
	if err := env.Load(&raw, nil); err != nil {
		log.Fatalf("failed to load litellm config: %v", err)
	}

	logger := log.New(os.Stdout, "", log.LstdFlags)

	httpClient := &http.Client{
		Timeout: raw.HTTPTimeout,
	}

	impl := New(httpClient, logger, config{
		baseURL: strings.TrimSpace(raw.BaseURL),
		apiKey:  strings.TrimSpace(raw.APIKey),
	})

	pluginmain.Run(impl, defaultPort, logger)
}
