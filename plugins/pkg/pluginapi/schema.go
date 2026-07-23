package pluginapi

const (
	NodeKindApplication       = "application"
	NodeKindDataset           = "dataset"
	NodeKindModel             = "model"
	NodeKindDeployment        = "deployment"
	NodeKindService           = "service"
	NodeKindFluxKustomization = "Kustomization.fluxcd"
	NodeKindFluxHelmChart     = "HelmChart.fluxcd"
	NodeKindGitRepository     = "git_repository"
	NodeKindHuggingFaceRepository = "hugging_face_repository"
)

const (
	RelationKindTrainedOn    = "trained_on"
	RelationKindUsesModel    = "uses_model"
	RelationKindCalls        = "calls"
	RelationKindSourcedFrom  = "sourced_from"
	RelationKindDescribes    = "describes"
	RelationKindDeployedFrom = "deployed_from"
	RelationKindDerivedFrom  = "derived_from"
)
