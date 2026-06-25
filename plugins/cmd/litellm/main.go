package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/naira-project/naira/plugins/pkg/pluginmain"
)

type config struct {
	BaseURL     string        `env:"LITELLM_BASE_URL" default:"http://127.0.0.1:4000"`
	APIKey      string        `env:"LITELLM_API_KEY"`
	HTTPTimeout time.Duration `env:"LITELLM_HTTP_TIMEOUT" default:"5s"`
}

func main() {
	logger := log.New(os.Stdout, "", log.LstdFlags)
	cfg, srvCfg := pluginmain.LoadConfig[config](logger)

	httpClient := &http.Client{Timeout: cfg.HTTPTimeout}
	p := New(httpClient, logger, cfg)

	pluginmain.Serve(p, srvCfg, logger)
}
