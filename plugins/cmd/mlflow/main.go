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

const defaultPort = 50052

type pluginConfig struct {
	BaseURL     string        `env:"MLFLOW_BASE_URL" default:"http://127.0.0.1:5000"`
	BearerToken string        `env:"MLFLOW_BEARER_TOKEN"`
	HTTPTimeout time.Duration `env:"MLFLOW_HTTP_TIMEOUT" default:"5s"`
}

func main() {
	var raw pluginConfig
	if err := env.Load(&raw, nil); err != nil {
		log.Fatalf("failed to load mlflow config: %v", err)
	}

	httpClient := &http.Client{
		Timeout: raw.HTTPTimeout,
	}

	logger := log.New(os.Stdout, "", log.LstdFlags)

	impl := New(httpClient, config{
		baseURL:     strings.TrimSpace(raw.BaseURL),
		bearerToken: strings.TrimSpace(raw.BearerToken),
	})

	pluginmain.Run(impl, defaultPort, logger)
}
