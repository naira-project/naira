package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/naira-project/naira/plugins/internal/pluginmain"
)

type config struct {
	BaseURL     string        `env:"MLFLOW_BASE_URL" default:"http://127.0.0.1:5000"`
	BearerToken string        `env:"MLFLOW_BEARER_TOKEN"`
	HTTPTimeout time.Duration `env:"MLFLOW_HTTP_TIMEOUT" default:"5s"`
}

func main() {
	logger := log.New(os.Stdout, "", log.LstdFlags)
	cfg, srvCfg := pluginmain.LoadConfig[config](logger)
	httpClient := &http.Client{Timeout: cfg.HTTPTimeout}
	p := New(httpClient, cfg)

	pluginmain.Serve(p, srvCfg, logger)
}
