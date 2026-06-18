package plugins

import (
	"log"
	"net/http"

	"github.com/naira-project/naira/catalog/internal/plugins/depl_calls_svc"
	"github.com/naira-project/naira/catalog/internal/plugins/litellm"
	"github.com/naira-project/naira/catalog/internal/plugins/mlflow"
	"github.com/naira-project/naira/catalog/internal/plugins/openmetadata"
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

	if config.OpenMetadata.Enabled {
		registered = appendIfNotNil(registered, openmetadata.New(httpClient, config.OpenMetadata))
	}

	return registered
}

func appendIfNotNil(plugins []pluginapi.Plugin, plugin pluginapi.Plugin) []pluginapi.Plugin {
	if plugin != nil {
		return append(plugins, plugin)
	}
	return plugins
}
