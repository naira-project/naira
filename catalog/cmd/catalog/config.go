package main

import (
	"fmt"
	"os"
	"time"

	"github.com/naira-project/naira/catalog/internal/catalog"

	"go-simpler.org/env"
	"gopkg.in/yaml.v3"
)

type config struct {
	Port                    int
	ReadHeadersTimeout      time.Duration
	ShutdownTimeout         time.Duration
	Plugins                 map[string]catalog.PluginDefinition
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
	PluginConfigFile        string        `env:"PLUGIN_CONFIG_FILE" default:"/etc/catalog/plugins.yaml"`
	PluginConnectionTimeout time.Duration `env:"PLUGIN_CONNECTION_TIMEOUT" default:"10s"`
	PluginTimeout           time.Duration `env:"PLUGIN_TIMEOUT" default:"5m"`
	KeycloakBaseURL         string        `env:"KEYCLOAK_BASE_URL"`
	KeycloakRealm           string        `env:"KEYCLOAK_REALM"`
	KeycloakIssuer          string        `env:"KEYCLOAK_ISSUER"`
}

type pluginConfig struct {
	Plugins map[string]pluginEntry `yaml:"plugins"`
}

type pluginEntry struct {
	Address  string `yaml:"address"`
	Schedule string `yaml:"schedule"`
}

func loadConfig() (config, error) {
	var raw envConfig
	if err := env.Load(&raw, nil); err != nil {
		return config{}, fmt.Errorf("load environment configuration: %w", err)
	}

	contents, err := os.ReadFile(raw.PluginConfigFile)
	if err != nil {
		return config{}, fmt.Errorf("read plugin configuration file %q: %w", raw.PluginConfigFile, err)
	}

	var plugins pluginConfig
	if err := yaml.Unmarshal(contents, &plugins); err != nil {
		return config{}, fmt.Errorf("parse plugin configuration file %q: %w", raw.PluginConfigFile, err)
	}
	pluginDefinitions := make(map[string]catalog.PluginDefinition, len(plugins.Plugins))
	for name, plugin := range plugins.Plugins {
		if plugin.Address == "" {
			return config{}, fmt.Errorf("plugin %q has no address", name)
		}
		pluginDefinitions[name] = catalog.PluginDefinition{Address: plugin.Address, Schedule: plugin.Schedule}
	}

	return config{
		Port:                    raw.Port,
		ReadHeadersTimeout:      raw.ReadHeadersTimeout,
		ShutdownTimeout:         raw.ShutdownTimeout,
		Plugins:                 pluginDefinitions,
		PluginConnectionTimeout: raw.PluginConnectionTimeout,
		PluginTimeout:           raw.PluginTimeout,
		KeycloakBaseURL:         raw.KeycloakBaseURL,
		KeycloakRealm:           raw.KeycloakRealm,
		KeycloakIssuer:          raw.KeycloakIssuer,
	}, nil
}
