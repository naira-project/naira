package main

import (
	"fmt"
	"strings"
	"time"

	"go-simpler.org/env"
)

type config struct {
	Port            int
	HTTPTimeout     time.Duration
	PluginAddresses map[string]string
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

	pluginAddresses, err := parsePluginAddresses(raw.PluginAddresses)
	if err != nil {
		return config{}, fmt.Errorf("parse plugin addresses: %w", err)
	}

	cfg := config{
		Port:            raw.Port,
		HTTPTimeout:     raw.HTTPTimeout,
		PluginAddresses: pluginAddresses,
	}

	return cfg, nil
}

func parsePluginAddresses(raw []string) (map[string]string, error) {
	plugins := make(map[string]string)
	for _, entry := range raw {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid plugin address entry %q: must be in name=address format", entry)
		}
		name := strings.TrimSpace(parts[0])
		addr := strings.TrimSpace(parts[1])
		if name == "" || addr == "" {
			return nil, fmt.Errorf("invalid plugin address entry %q: name and address must not be empty", entry)
		}
		plugins[name] = addr
	}
	return plugins, nil
}
