package main

import (
	"fmt"
	"time"

	"go-simpler.org/env"
)

type config struct {
	Port            int
	HTTPTimeout     time.Duration
	PluginAddresses []string
}

type envConfig struct {
	Port            int           `env:"PORT" default:"8090"`
	HTTPTimeout     time.Duration `env:"HTTP_TIMEOUT" default:"5s"`
	PluginAddresses []string      `env:"PLUGIN_ADDRESSES"`
}

func loadConfig() (config, error) {
	var raw envConfig
	opts := &env.Options{
		SliceSep: ",",
	}
	if err := env.Load(&raw, opts); err != nil {
		return config{}, fmt.Errorf("load config from environment: %w", err)
	}

	cfg := config{
		Port:            raw.Port,
		HTTPTimeout:     raw.HTTPTimeout,
		PluginAddresses: raw.PluginAddresses,
	}

	return cfg, nil
}
