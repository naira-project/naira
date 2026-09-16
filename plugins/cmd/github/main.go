// github finds GitHub repositories whose artifact attestations cryptographically
// prove that they built images running in Kubernetes Deployments, then enriches
// these repositories with metadata and CODEOWNERS information.
//
// A Deployment is linked to a repository only after verification with "gh attestation verify".
//
// Verification runs only for image references mentioning GITHUB_ORG, to avoid
// unnecessary gh calls.
//
// Deployments with more than one container are not verified because the
// repository cannot be attributed to a single image unambiguously.
//
// TODO: Link deployments with more than one container to source
// repositories, verifying each container image independently.
//
// TODO: Check support for private OCI registries and private GitHub repositories.
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
//go:generate bash -c "goreadme -use-stdlib-markdown -title 'github plugin' | sed 's/ {#hdr-[^}]*}//g' > README.md"
package main

import (
	"context"
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

	GitHubOrg          string        `env:"GITHUB_ORG"`
	GitHubToken        string        `env:"GITHUB_TOKEN"`
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
	if app.PluginConfig.GitHubOrg == "" {
		app.Logger.Fatal("GITHUB_ORG is required")
	}
	if app.PluginConfig.GitHubToken == "" {
		app.Logger.Fatal("GITHUB_TOKEN is required")
	}
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

	entries, err := discoverDeployments(ctx, k8sClient, p.logger)
	if err != nil {
		return pluginapi.CollectResponse{}, fmt.Errorf("discovering deployments: %w", err)
	}

	var resp pluginapi.CollectResponse
	repos := newRepoCache()

	for _, entry := range entries {
		image, ok := singleContainerImage(entry)
		if !ok {
			// Ambiguous attribution with more than one container
			continue
		}
		if !imageReferencesOrg(image, p.config.GitHubOrg) {
			continue
		}

		owner, name, err := p.attestation.Verify(ctx, image, p.config.GitHubOrg)
		if err != nil {
			p.logger.Printf("verifying attestation for %s (deployment %s/%s): %v", image, entry.Namespace, entry.Name, err)
			continue
		}

		repoNodeID := gitRepositoryNodeID(owner, name)

		if !repos.AlreadyCollected(repoNodeID) {
			nodes, relations, err := p.collectRepo(ctx, owner, name)
			if err != nil {
				p.logger.Printf("collecting repo %s/%s, err: %v", owner, name, err)
				continue
			}
			resp.Nodes = append(resp.Nodes, nodes...)
			resp.Relations = append(resp.Relations, relations...)
			repos.MarkCollected(repoNodeID)
		}

		deploymentNodeClaim := pluginapi.NodeClaim{ID: entry.NodeID()}
		deploymentRepoRelation := pluginapi.RelationClaim{
			Kind: pluginapi.RelationKindBuiltFrom,
			From: entry.NodeID(),
			To:   repoNodeID,
		}
		resp.Nodes = append(resp.Nodes, deploymentNodeClaim)
		resp.Relations = append(resp.Relations, deploymentRepoRelation)
	}

	return resp, nil
}

func singleContainerImage(entry Deployment) (image string, ok bool) {
	if len(entry.Images) != 1 {
		return "", false
	}
	return entry.Images[0], true
}

// imageReferencesOrg is a filter to skip the network calls for images that are not in
// the configured org. It is not a security check - it's an optimization
func imageReferencesOrg(image, org string) bool {
	if org == "" {
		return false
	}
	return strings.Contains(strings.ToLower(image), strings.ToLower(org))
}

func (p *Plugin) collectRepo(ctx context.Context, owner, name string) ([]pluginapi.NodeClaim, []pluginapi.RelationClaim, error) {
	repo, found, err := p.github.GetRepo(ctx, owner, name)
	if err != nil {
		return nil, nil, fmt.Errorf("fetching repo: %w", err)
	}
	if !found {
		return nil, nil, fmt.Errorf("repo not found or not accessible")
	}

	repoNodeID := gitRepositoryNodeID(owner, name)
	props := pluginapi.PropertyMap{
		propertyKeyURL: repo.HTMLURL,
	}
	if repo.Language != "" {
		props[propertyKeyLanguage] = repo.Language
	}
	if repo.Homepage != "" {
		props[propertyKeyHomepage] = repo.Homepage
	}
	nodes := []pluginapi.NodeClaim{
		{ID: repoNodeID, Properties: props},
	}

	codeowners, found, err := p.github.GetCodeowners(ctx, owner, name)
	if err != nil {
		return nil, nil, fmt.Errorf("fetching codeowners: %w", err)
	}
	if !found {
		return nodes, nil, nil
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

func gitRepositoryNodeID(owner, name string) pluginapi.NodeID {
	return pluginapi.NodeID{
		Kind: pluginapi.NodeKindGitRepository,
		Path: repositoryidentity.GitHubRepositoryNodePath(owner, name),
	}
}

type repoCache struct {
	collected map[string]bool // node path -> already collected
}

func newRepoCache() *repoCache {
	return &repoCache{collected: make(map[string]bool)}
}

func (c *repoCache) AlreadyCollected(id pluginapi.NodeID) bool {
	return c.collected[id.Path]
}

func (c *repoCache) MarkCollected(id pluginapi.NodeID) {
	c.collected[id.Path] = true
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
