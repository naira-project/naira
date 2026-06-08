package plugins

import (
	"log"
	"net/http"

	"github.com/naira-project/naira/catalog/internal/plugins/depl_calls_svc"
	"github.com/naira-project/naira/catalog/internal/plugins/depl_uses_litellm"
	"github.com/naira-project/naira/catalog/internal/plugins/fluxcd"
	"github.com/naira-project/naira/catalog/internal/plugins/litellm"
	"github.com/naira-project/naira/catalog/internal/plugins/mlflow"
	"github.com/naira-project/naira/catalog/pluginapi"
)

func Register(config Config, httpClient *http.Client, logger *log.Logger) []pluginapi.Plugin {
	var registered []pluginapi.Plugin

	if config.MLflow.Enabled {
		registered = appendIfNotNil(registered, mlflow.New(httpClient, config.MLflow))
	}

	if config.LiteLLM.Enabled {
		registered = appendIfNotNil(registered, litellm.New(httpClient, logger, config.LiteLLM))
	}

	if config.DeplCallsSvc.Enabled {
		registered = appendIfNotNil(registered, depl_calls_svc.New(config.DeplCallsSvc))
	}

	if config.Fluxcd.Enabled {
		registered = appendIfNotNil(registered, fluxcd.New(config.Fluxcd))
	}

	if config.DeplUsesLiteLLM.Enabled {
		plugin, err := depl_uses_litellm.New(httpClient, config.DeplUsesLiteLLM)
		if err != nil {
			logger.Printf("error initializing DeplLiteLLMModels plugin: %v", err)
		}
		registered = appendIfNotNil(registered, plugin)
	}

	return registered
}

func appendIfNotNil(plugins []pluginapi.Plugin, plugin pluginapi.Plugin) []pluginapi.Plugin {
	if plugin != nil {
		return append(plugins, plugin)
	}
	return plugins
}
