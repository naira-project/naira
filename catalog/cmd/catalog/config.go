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
	Port              int                                `env:"PORT" default:"8090"`
	HTTPTimeout       time.Duration                      `env:"HTTP_TIMEOUT" default:"5s"`
	MLflow            plugins.MLflowEnvConfig            `env:"MLFLOW_"`
	LiteLLM           plugins.LiteLLMEnvConfig           `env:"LITELLM_"`
	DeplSvcsCalls     plugins.DeplSvcsCallsEnvConfig     `env:"DEPL_SVCS_CALLS_"`
	FluxcdDeploys     plugins.FluxcdDeploysEnvConfig     `env:"FLUXCD_DEPLOYS_"`
	DeplLiteLLMModels plugins.DeplLiteLLMModelsEnvConfig `env:"DEPL_LITELLM_MODELS_"`
}

func loadConfig() (config, error) {
	var raw envConfig
	if err := env.Load(&raw, nil); err != nil {
		return config{}, fmt.Errorf("load config from environment: %w", err)
	}

	cfg := config{
		Port:        raw.Port,
		HTTPTimeout: raw.HTTPTimeout,
		Plugins:     plugins.LoadConfig(raw.MLflow, raw.LiteLLM, raw.DeplSvcsCalls, raw.FluxcdDeploys, raw.DeplLiteLLMModels),
	}

	return cfg, nil
}
