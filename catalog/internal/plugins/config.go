package plugins

import (
	"strings"

	deplsvcscalls "github.com/naira-project/naira/catalog/internal/plugins/depl_svcs_calls"
	"github.com/naira-project/naira/catalog/internal/plugins/litellm"
	"github.com/naira-project/naira/catalog/internal/plugins/mlflow"
)

type Config struct {
	MLflow        mlflow.Config
	LiteLLM       litellm.Config
	DeplSvcsCalls deplsvcscalls.Config
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

func LoadConfig(mlflowConfig MLflowEnvConfig, litellmConfig LiteLLMEnvConfig, deplSvcsCallsConfig DeplSvcsCallsEnvConfig) Config {
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
	}
}
