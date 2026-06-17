package plugins

import (
	"github.com/naira-project/naira/catalog/internal/plugins/depl_calls_svc"
	"github.com/naira-project/naira/catalog/internal/plugins/litellm"
	"github.com/naira-project/naira/catalog/internal/plugins/mlflow"
)

type Config struct {
	MLflow       mlflow.Config         `env:"MLFLOW_"`
	LiteLLM      litellm.Config        `env:"LITELLM_"`
	DeplCallsSvc depl_calls_svc.Config `env:"DEPL_CALLS_SVC_"`
}
