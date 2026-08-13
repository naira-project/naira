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

	// NodeKindOwner represents a person, team, or group referenced as an
	// owner of some other entity (e.g. a git_repository via CODEOWNERS).
	// Path is the raw owner handle as it appears in the source system,
	// e.g. "@platform-team" or "jane.doe@example.com".
	NodeKindOwner = "owner"
)

const (
	RelationKindTrainedOn    = "trained_on"
	RelationKindUsesModel    = "uses_model"
	RelationKindCalls        = "calls"
	RelationKindSourcedFrom  = "sourced_from"
	RelationKindDescribes    = "describes"
	RelationKindDeployedFrom = "deployed_from"
	RelationKindDerivedFrom  = "derived_from"

	// RelationKindOwnedBy links an entity (e.g. git_repository) to the
	// owner.NodeKindOwner node(s) responsible for it.
	RelationKindOwnedBy = "owned_by"
)
