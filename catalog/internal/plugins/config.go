package plugins

import (
	deplitellmmodels "github.com/naira-project/naira/catalog/internal/plugins/depl_litellm_models"
	deplsvcscalls "github.com/naira-project/naira/catalog/internal/plugins/depl_svcs_calls"
	fluxcddeploys "github.com/naira-project/naira/catalog/internal/plugins/fluxcd_deploys"
	"github.com/naira-project/naira/catalog/internal/plugins/litellm"
	"github.com/naira-project/naira/catalog/internal/plugins/mlflow"
)

type Config struct {
	MLflow            mlflow.Config           `env:"MLFLOW_"`
	LiteLLM           litellm.Config          `env:"LITELLM_"`
	DeplSvcsCalls     deplsvcscalls.Config    `env:"DEPL_SVCS_CALLS_"`
	FluxcdDeploys     fluxcddeploys.Config    `env:"FLUXCD_DEPLOYS_"`
	DeplLiteLLMModels deplitellmmodels.Config `env:"DEPL_LITELLM_MODELS_"`
}
