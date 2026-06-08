package main

import (
	"fmt"
	"time"

	"go-simpler.org/env"
)

type config struct {
	Port        int
	HTTPTimeout time.Duration
	PluginsDir  string
}

type envConfig struct {
	Port        int           `env:"PORT" default:"8090"`
	HTTPTimeout time.Duration `env:"HTTP_TIMEOUT" default:"5s"`
	PluginsDir  string        `env:"PLUGINS_DIR" default:"/var/lib/naira/plugins"`
}

func loadConfig() (config, error) {
	var raw envConfig
	if err := env.Load(&raw, nil); err != nil {
		return config{}, fmt.Errorf("load config from environment: %w", err)
	}

	cfg := config{
		Port:        raw.Port,
		HTTPTimeout: raw.HTTPTimeout,
		PluginsDir:  raw.PluginsDir,
	}

	return cfg, nil
}
