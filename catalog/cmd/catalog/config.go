package main

import (
	"fmt"
	"time"

	"github.com/naira-project/naira/catalog/internal/plugins"

	"go-simpler.org/env"
)

type config struct {
	Port        int
	HTTPTimeout time.Duration
	Plugins     plugins.Config
}

type envConfig struct {
	Port        int           `env:"PORT" default:"8090"`
	HTTPTimeout time.Duration `env:"HTTP_TIMEOUT" default:"5s"`
	Plugins     plugins.Config
}

func loadConfig() (config, error) {
	var raw envConfig
	if err := env.Load(&raw, &env.Options{SliceSep: ","}); err != nil {
		return config{}, fmt.Errorf("load config from environment: %w", err)
	}

	return config{
		Port:        raw.Port,
		HTTPTimeout: raw.HTTPTimeout,
		Plugins:     raw.Plugins,
	}, nil
}
