package plugins

import (
	"log"
	"net/http"

	deplsvcscalls "github.com/naira-project/naira/catalog/internal/plugins/depl_svcs_calls"
	fluxcddeploys "github.com/naira-project/naira/catalog/internal/plugins/fluxcd_deploys"
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

	if config.DeplSvcsCalls.Enabled {
		registered = appendIfNotNil(registered, deplsvcscalls.New(config.DeplSvcsCalls))
	}

	if config.FluxcdDeploys.Enabled {
		registered = appendIfNotNil(registered, fluxcddeploys.New(config.FluxcdDeploys))
	}

	return registered
}

func appendIfNotNil(plugins []pluginapi.Plugin, plugin pluginapi.Plugin) []pluginapi.Plugin {
	if plugin != nil {
		return append(plugins, plugin)
	}
	return plugins
}
