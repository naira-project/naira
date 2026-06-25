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

type config struct {
	BaseURL     string        `env:"LITELLM_BASE_URL" default:"http://127.0.0.1:4000"`
	APIKey      string        `env:"LITELLM_API_KEY"`
	HTTPTimeout time.Duration `env:"LITELLM_HTTP_TIMEOUT" default:"5s"`
}

func main() {
	var raw config
	if err := env.Load(&raw, nil); err != nil {
		log.Fatalf("failed to load litellm config: %v", err)
	}

	logger := log.New(os.Stdout, "", log.LstdFlags)

	httpClient := &http.Client{
		Timeout: raw.HTTPTimeout,
	}

	impl := New(httpClient, logger, config{
		BaseURL: strings.TrimSpace(raw.BaseURL),
		APIKey:  strings.TrimSpace(raw.APIKey),
	})

	pluginmain.Run(impl, logger)
}
