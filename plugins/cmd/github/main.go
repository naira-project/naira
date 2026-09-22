// github plugin connects Kubernetes Deployments to their source GitHub repositories.
//
// It discovers repositories whose artifact attestations cryptographically prove
// that they built the container images running in a cluster, enriching the results
// with repository metadata and top-level CODEOWNERS ownership.
//
// # How It Works
//
//   - Attestation Verification: A Deployment is linked to a repository only after
//     successful verification using `gh attestation verify`.
//   - ghcr.io Optimization: Images on ghcr.io that do not belong to GITHUB_ORG
//     are skipped to avoid unnecessary CLI calls. Non-ghcr.io images are always
//     verified since path conventions vary across registries.
//   - Single-Container Limit: Deployments with multiple containers are skipped
//     because ownership cannot be attributed to a single image unambiguously.
//   - CODEOWNERS Attribution: Only the default (`*`) rule is used to assign ownership.
//     Path-specific rules (e.g., `/docs/ @docs-team`) are ignored to prevent misrepresenting
//     partial owners as whole-repository owners.
//
// # Environment Variables
//
//   - GITHUB_ORG (mandatory) - limits collection to repositories whose
//     attestations are verified for this GitHub organization, via
//     "gh attestation verify --owner".
//   - GITHUB_TOKEN (mandatory) - GitHub API token used for repository
//     metadata and CODEOWNERS, and passed as GH_TOKEN to "gh attestation verify".
//     It is required even for public repositories.
//     A classic token with no selected scopes is sufficient.
//   - GITHUB_BASE_URL (optional) - GitHub API base URL; defaults to
//     "https://api.github.com".
//   - GITHUB_HTTP_TIMEOUT (optional) - GitHub API request timeout; defaults
//     to "10s".
//   - GITHUB_ATTESTATION_TIMEOUT (optional) - timeout for a single
//     "gh attestation verify" invocation; defaults to "30s".
//   - GH_CLI_PATH (optional) - path to the "gh" binary; defaults to "gh"
//     (resolved from PATH).
//   - KUBECONFIG (optional) - path to a kubeconfig file; when unset,
//     in-cluster configuration is used.
//
// # TODOs
//
//   - Link multi-container deployments by verifying each image independently.
//   - Add registry filtering for non-ghcr.io images (e.g., `AllowedImagePrefixes`).
//   - Verify support for private OCI registries and private GitHub repositories.
//
//go:generate bash -c "goreadme -use-stdlib-markdown -title 'github plugin' | sed 's/ {#hdr-[^}]*}//g' > README.md"
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/naira-project/naira/plugins/internal/kubeutil"
	"github.com/naira-project/naira/plugins/internal/repositoryidentity"
	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/naira-project/naira/plugins/pkg/pluginmain"
	"k8s.io/client-go/kubernetes"
)

const (
	// git_repository property keys
	propertyKeyURL      = "url"
	propertyKeyLanguage = "language"
	propertyKeyHomepage = "homepage"
)

type config struct {
	Kubeconfig string `env:"KUBECONFIG"`

	GitHubOrg          string        `env:"GITHUB_ORG,required"`
	GitHubToken        string        `env:"GITHUB_TOKEN,required"`
	GitHubBaseURL      string        `env:"GITHUB_BASE_URL" default:"https://api.github.com"`
	HTTPTimeout        time.Duration `env:"GITHUB_HTTP_TIMEOUT" default:"10s"`
	GHCLIPath          string        `env:"GH_CLI_PATH" default:"gh"`
	AttestationTimeout time.Duration `env:"GITHUB_ATTESTATION_TIMEOUT" default:"30s"`
}

type Plugin struct {
	github      *githubClient
	attestation *attestationVerifier
	logger      *log.Logger
	config      config
}

func New(config config, logger *log.Logger) *Plugin {
	return &Plugin{
		github:      newGithubClient(&http.Client{Timeout: config.HTTPTimeout}, config.GitHubBaseURL, config.GitHubToken),
		attestation: newAttestationVerifier(config.GHCLIPath, config.GitHubToken, config.AttestationTimeout),
		logger:      logger,
		config:      config,
	}
}

func main() {
	app := pluginmain.New[config]()
	p := New(app.PluginConfig, app.Logger)
	app.Serve(p)
}

func (p *Plugin) Collect(ctx context.Context) (pluginapi.CollectResponse, error) {
	k8sClient, err := p.connect()
	if err != nil {
		return pluginapi.CollectResponse{}, fmt.Errorf("connecting to cluster: %w", err)
	}
	return p.collect(ctx, k8sClient)
}

func (p *Plugin) collect(ctx context.Context, k8sClient kubernetes.Interface) (pluginapi.CollectResponse, error) {
	if err := p.attestation.CheckGhAvailable(ctx); err != nil {
		return pluginapi.CollectResponse{}, fmt.Errorf("checking gh CLI availability: %w", err)
	}

	deployments, err := discoverDeployments(ctx, k8sClient, p.logger)
	if err != nil {
		return pluginapi.CollectResponse{}, fmt.Errorf("discovering deployments: %w", err)
	}

	var resp pluginapi.CollectResponse
	repos := make(map[ownerAndName]bool)

	for _, deployment := range deployments {
		image, ok := singleContainerImage(deployment)
		if !ok {
			// Ambiguous attribution with more than one container
			continue
		}
		if !p.shouldVerifyImage(image) {
			continue
		}

		repo, err := p.attestation.Verify(ctx, image, p.config.GitHubOrg)
		if err != nil {
			p.logger.Printf("verifying attestation for %s (deployment %s/%s): %v", image, deployment.Namespace, deployment.Name, err)
			continue
		}

		if !repos[repo] {
			nodes, relations, err := p.collectRepo(ctx, repo)
			if err != nil {
				p.logger.Printf("collecting repo %s/%s, err: %v", repo.owner, repo.name, err)
				continue
			}
			resp.Nodes = append(resp.Nodes, nodes...)
			resp.Relations = append(resp.Relations, relations...)
			repos[repo] = true
		}

		deploymentNodeClaim := pluginapi.NodeClaim{ID: deployment.NodeID()}
		deploymentRepoRelation := pluginapi.RelationClaim{
			Kind: pluginapi.RelationKindBuiltFrom,
			From: deployment.NodeID(),
			To:   repo.ToNodeID(),
		}
		resp.Nodes = append(resp.Nodes, deploymentNodeClaim)
		resp.Relations = append(resp.Relations, deploymentRepoRelation)
	}

	return resp, nil
}

func singleContainerImage(deployment Deployment) (image string, ok bool) {
	if len(deployment.Images) != 1 {
		return "", false
	}
	return deployment.Images[0], true
}

// shouldVerifyImage reports whether the given image reference is a candidate
// for attestation verification based on its registry prefix.
//
// ghcr.io images follow a predictable ghcr.io/<owner>/<repo> layout, so we
// can cheaply filter out images that clearly belong to a different GitHub
// org without spawning "gh". Other registries don't share this convention
func (p *Plugin) shouldVerifyImage(image string) bool {
	imageLower := strings.ToLower(image)

	if !strings.HasPrefix(imageLower, "ghcr.io/") {
		return true
	}
	orgLower := strings.ToLower(p.config.GitHubOrg)
	return strings.HasPrefix(imageLower, "ghcr.io/"+orgLower+"/")
}

func (p *Plugin) collectRepo(ctx context.Context, repo ownerAndName) ([]pluginapi.NodeClaim, []pluginapi.RelationClaim, error) {
	githubRepo, err := p.github.GetRepo(ctx, repo.owner, repo.name)
	if err != nil {
		return nil, nil, fmt.Errorf("fetching repo: %w", err)
	}

	repoNodeID := repo.ToNodeID()
	props := pluginapi.PropertyMap{
		propertyKeyURL: githubRepo.HTMLURL,
	}
	if githubRepo.Language != "" {
		props[propertyKeyLanguage] = githubRepo.Language
	}
	if githubRepo.Homepage != "" {
		props[propertyKeyHomepage] = githubRepo.Homepage
	}
	nodes := []pluginapi.NodeClaim{
		{ID: repoNodeID, Properties: props},
	}

	codeowners, err := p.github.GetCodeowners(ctx, repo.owner, repo.name)
	if errors.Is(err, errGithubResourceNotFound) {
		return nodes, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("fetching codeowners: %w", err)
	}

	ownerNodes, ownedByRelations := codeownersClaims(repoNodeID, extractDefaultCodeowners(codeowners))
	nodes = append(nodes, ownerNodes...)
	return nodes, ownedByRelations, nil
}

func codeownersClaims(repoNodeID pluginapi.NodeID, handles []string) ([]pluginapi.NodeClaim, []pluginapi.RelationClaim) {
	nodes := make([]pluginapi.NodeClaim, 0, len(handles))
	relations := make([]pluginapi.RelationClaim, 0, len(handles))

	for _, handle := range handles {
		ownerNodeID := pluginapi.NodeID{Kind: pluginapi.NodeKindOwner, Path: handle}
		nodes = append(nodes, pluginapi.NodeClaim{ID: ownerNodeID})
		relations = append(relations, pluginapi.RelationClaim{
			Kind: pluginapi.RelationKindOwnedBy,
			From: repoNodeID,
			To:   ownerNodeID,
		})
	}

	return nodes, relations
}

type ownerAndName struct {
	owner string
	name  string
}

func (r ownerAndName) ToNodeID() pluginapi.NodeID {
	return pluginapi.NodeID{
		Kind: pluginapi.NodeKindGitRepository,
		Path: repositoryidentity.GitHubRepositoryNodePath(r.owner, r.name),
	}
}

func (p *Plugin) connect() (*kubernetes.Clientset, error) {
	cfg, err := kubeutil.RestConfig(p.config.Kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("loading k8s config: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}
	return clientset, nil
}
