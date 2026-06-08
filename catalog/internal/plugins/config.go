package plugins

import (
	"strings"

	"github.com/naira-project/naira/catalog/internal/plugins/litellm"
	"github.com/naira-project/naira/catalog/internal/plugins/mlflow"
)

type Config struct {
	MLflow     mlflow.Config
	LiteLLM    litellm.Config
	PluginsDir string
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

func LoadConfig(mlflowConfig MLflowEnvConfig, litellmConfig LiteLLMEnvConfig, pluginsDir string) Config {
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
		PluginsDir: strings.TrimSpace(pluginsDir),
	}
}

