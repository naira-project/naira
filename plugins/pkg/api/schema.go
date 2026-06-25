package pluginapi

const (
	NodeKindApplication = "application"
	NodeKindDataset     = "dataset"
	NodeKindModel       = "model"
	NodeKindDeployment  = "deployment"
	NodeKindService     = "service"
)

const (
	RelationKindTrainedOn = "trained_on"
	RelationKindUsesModel = "uses_model"
	RelationKindCalls     = "calls"
)
