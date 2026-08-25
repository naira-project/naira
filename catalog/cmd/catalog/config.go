package main

import (
	"fmt"
	"strings"
	"time"

	"go-simpler.org/env"
)

type config struct {
	Port                    int
	ReadHeadersTimeout      time.Duration
	ShutdownTimeout         time.Duration
	PluginAddresses         map[string]string
	PluginSchedules         map[string]string
	PluginConnectionTimeout time.Duration
	PluginTimeout           time.Duration
	KeycloakBaseURL         string
	KeycloakRealm           string
	KeycloakIssuer          string
}

type envConfig struct {
	Port                    int           `env:"PORT" default:"8090"`
	ReadHeadersTimeout      time.Duration `env:"READ_HEADERS_TIMEOUT" default:"5s"`
	ShutdownTimeout         time.Duration `env:"SHUTDOWN_TIMEOUT" default:"5s"`
	PluginAddresses         []string      `env:"PLUGIN_ADDRESSES"`
	PluginSchedules         []string      `env:"PLUGIN_SCHEDULES"`
	PluginConnectionTimeout time.Duration `env:"PLUGIN_CONNECTION_TIMEOUT" default:"10s"`
	PluginTimeout           time.Duration `env:"PLUGIN_TIMEOUT" default:"5m"`
	KeycloakBaseURL         string        `env:"KEYCLOAK_BASE_URL"`
	KeycloakRealm           string        `env:"KEYCLOAK_REALM"`
	KeycloakIssuer          string        `env:"KEYCLOAK_ISSUER"`
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
	pluginSchedules, err := parsePluginSchedules(raw.PluginSchedules)
	if err != nil {
		return config{}, fmt.Errorf("parse plugin schedules: %w", err)
	}

	cfg := config{
		Port:                    raw.Port,
		ReadHeadersTimeout:      raw.ReadHeadersTimeout,
		ShutdownTimeout:         raw.ShutdownTimeout,
		PluginAddresses:         pluginAddresses,
		PluginSchedules:         pluginSchedules,
		PluginConnectionTimeout: raw.PluginConnectionTimeout,
		PluginTimeout:           raw.PluginTimeout,
		KeycloakBaseURL:         raw.KeycloakBaseURL,
		KeycloakRealm:           raw.KeycloakRealm,
		KeycloakIssuer:          raw.KeycloakIssuer,
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
		name, addr, ok := strings.Cut(entry, "=")
		if !ok {
			return nil, fmt.Errorf("invalid plugin address entry %q: must be in name=address format", entry)
		}
		name = strings.TrimSpace(name)
		addr = strings.TrimSpace(addr)
		if name == "" || addr == "" {
			return nil, fmt.Errorf("invalid plugin address entry %q: name and address must not be empty", entry)
		}
		plugins[name] = addr
	}
	return plugins, nil
}

// parsePluginSchedules reads optional default schedules in plugin=cron format.
// A missing entry means that the plugin is manual-only.
func parsePluginSchedules(raw []string) (map[string]string, error) {
	schedules := make(map[string]string)
	for _, entry := range raw {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		name, expression, ok := strings.Cut(entry, "=")
		name = strings.TrimSpace(name)
		expression = strings.TrimSpace(expression)
		if !ok || name == "" || expression == "" {
			return nil, fmt.Errorf("invalid plugin schedule entry %q: must be in name=cron format", entry)
		}
		schedules[name] = expression
	}
	return schedules, nil
}
