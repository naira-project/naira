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
	KeycloakBaseURL string
	KeycloakRealm   string
	KeycloakClient  string
	OpenfgaBaseURL 	string
	OpenfgaSchemaPath string
	OpenfgaStoreName string
}

type envConfig struct {
	Port        int           `env:"PORT" default:"8090"`
	HTTPTimeout time.Duration `env:"HTTP_TIMEOUT" default:"5s"`
	Plugins     plugins.Config
	KeycloakBaseURL string `env:"KEYCLOAK_BASE_URL"`
	KeycloakRealm   string `env:"KEYCLOAK_REALM"`
	KeycloakClient  string `env:"KEYCLOAK_CLIENT"`
	OpenfgaBaseURL 	string	`env:"FGA_API_URL"`
	OpenfgaSchemaPath string `env:"FGA_SCHEMA_PATH"`
	OpenfgaStoreName  string `env:"FGA_STORE_NAME"`
}


func loadConfig() (config, error) {
	var raw envConfig
	if err := env.Load(&raw, nil); err != nil {
		return config{}, fmt.Errorf("load config from environment: %w", err)
	}

	return config{
		Port:        raw.Port,
		HTTPTimeout: raw.HTTPTimeout,
		Plugins:     raw.Plugins,
		KeycloakBaseURL: raw.KeycloakBaseURL,
		KeycloakRealm:   raw.KeycloakRealm,
		KeycloakClient:  raw.KeycloakClient,
		OpenfgaBaseURL:	 raw.OpenfgaBaseURL,
		OpenfgaSchemaPath: raw.OpenfgaSchemaPath,
		OpenfgaStoreName: raw.OpenfgaStoreName,
	}, nil
}
