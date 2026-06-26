package main

import (
	"net/http"
	"time"

	"github.com/naira-project/naira/plugins/pkg/pluginmain"
)

type config struct {
	BaseURL     string        `env:"MLFLOW_BASE_URL" default:"http://127.0.0.1:5000"`
	BearerToken string        `env:"MLFLOW_BEARER_TOKEN"`
	HTTPTimeout time.Duration `env:"MLFLOW_HTTP_TIMEOUT" default:"5s"`
}

func main() {
	app := pluginmain.New[config]()
	cfg := app.Config()

	httpClient := &http.Client{Timeout: cfg.HTTPTimeout}
	p := New(httpClient, cfg)

	app.Serve(p)
}
