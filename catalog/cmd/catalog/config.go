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
	Port               int           `env:"PORT" default:"8090"`
	ReadHeadersTimeout time.Duration `env:"READ_HEADERS_TIMEOUT" default:"5s"`
	ShutdownTimeout    time.Duration `env:"SHUTDOWN_TIMEOUT" default:"5s"`
	// PluginAddresses is a list of plugin name-to-address mappings.
	// Values must be in the format: name=address (comma-separated).
	// 	Example: PLUGIN_ADDRESSES="litellm=localhost:9001,mlflow=localhost:9002"
	PluginAddresses []string `env:"PLUGIN_ADDRESSES"`
	// PluginSchedules is an optional list of plugin cron schedules.
	// Values must be in the format: name=cron_expression (comma-separated).
	// A missing entry means the plugin is manual-only.
	// 	Example: PLUGIN_SCHEDULES="litellm=0 * * * *,mlflow=@daily"
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

	pluginAddresses, err := parseKeyValuePairs(raw.PluginAddresses)
	if err != nil {
		return config{}, fmt.Errorf("parse plugin addresses: %w", err)
	}

	pluginSchedules, err := parseKeyValuePairs(raw.PluginSchedules)
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

func parseKeyValuePairs(raw []string) (map[string]string, error) {
	result := make(map[string]string, len(raw))
	for _, entry := range raw {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		key, val, ok := strings.Cut(entry, "=")
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if !ok || key == "" || val == "" {
			return nil, fmt.Errorf("invalid entry %q: must be in key=value format", entry)
		}
		result[key] = val
	}
	return result, nil
}
