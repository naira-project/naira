package plugins

import (
	"strings"

	deplsvcscalls "github.com/naira-project/naira/catalog/internal/plugins/depl_svcs_calls"
	fluxcddeploys "github.com/naira-project/naira/catalog/internal/plugins/fluxcd_deploys"
	"github.com/naira-project/naira/catalog/internal/plugins/litellm"
	"github.com/naira-project/naira/catalog/internal/plugins/mlflow"
)

type Config struct {
	MLflow        mlflow.Config
	LiteLLM       litellm.Config
	DeplSvcsCalls deplsvcscalls.Config
	FluxcdDeploys fluxcddeploys.Config
}

type MLflowEnvConfig struct {
	Enabled     bool   `env:"ENABLED" default:"true"`
	BaseURL     string `env:"BASE_URL" default:"http://127.0.0.1:5000"`
	BearerToken string `env:"BEARER_TOKEN"`
}

type LiteLLMEnvConfig struct {
	Enabled bool   `env:"ENABLED" default:"true"`
	BaseURL string `env:"BASE_URL" default:"http://127.0.0.1:4000"`
	APIKey  string `env:"API_KEY"`
}

type DeplSvcsCallsEnvConfig struct {
	Enabled    bool   `env:"ENABLED" default:"true"`
	Kubeconfig string `env:"KUBECONFIG"`
	Namespace  string `env:"NAMESPACE"`
}

type FluxcdDeploysEnvConfig struct {
	Enabled    bool   `env:"ENABLED" default:"true"`
	Kubeconfig string `env:"KUBECONFIG"`
	Namespace  string `env:"NAMESPACE"`
}

func LoadConfig(mlflowConfig MLflowEnvConfig, litellmConfig LiteLLMEnvConfig, deplSvcsCallsConfig DeplSvcsCallsEnvConfig, fluxcdDeploysConfig FluxcdDeploysEnvConfig) Config {
	return Config{
		MLflow: mlflow.Config{
			Enabled:     mlflowConfig.Enabled,
			BaseURL:     strings.TrimSpace(mlflowConfig.BaseURL),
			BearerToken: strings.TrimSpace(mlflowConfig.BearerToken),
		},
		LiteLLM: litellm.Config{
			Enabled: litellmConfig.Enabled,
			BaseURL: strings.TrimSpace(litellmConfig.BaseURL),
			APIKey:  strings.TrimSpace(litellmConfig.APIKey),
		},
		DeplSvcsCalls: deplsvcscalls.Config{
			Enabled:    deplSvcsCallsConfig.Enabled,
			Kubeconfig: strings.TrimSpace(deplSvcsCallsConfig.Kubeconfig),
			Namespace:  strings.TrimSpace(deplSvcsCallsConfig.Namespace),
		},
		FluxcdDeploys: fluxcddeploys.Config{
			Enabled:    fluxcdDeploysConfig.Enabled,
			Kubeconfig: strings.TrimSpace(fluxcdDeploysConfig.Kubeconfig),
			Namespace:  strings.TrimSpace(fluxcdDeploysConfig.Namespace),
		},
	}
}
