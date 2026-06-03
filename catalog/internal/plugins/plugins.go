package plugins

import (
	"log"
	"net/http"

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

	return registered
}

func appendIfNotNil(plugins []pluginapi.Plugin, plugin pluginapi.Plugin) []pluginapi.Plugin {
	if plugin != nil {
		return append(plugins, plugin)
	}
	return plugins
}
