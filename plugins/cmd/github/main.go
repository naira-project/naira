// github discovers GitHub repositories that GitHub artifact attestations
// cryptographically prove built the images running in Kubernetes
// Deployments, then enriches those repositories with metadata and
// CODEOWNERS ownership information.
//
// Unlike depl_from_repo (which links Deployments to repositories using
// unverified, self-reported signals such as an OCI label or a registry-name
// guess), this plugin only ever links a Deployment to a repository it has
// verified via `gh attestation verify` against a GitHub artifact
// attestation. The resulting built_from relation is therefore a proven
// fact, not a guess.
//
// Attestation verification is attempted only for images whose reference
// mentions the configured GITHUB_ORG (a cheap pre-filter to avoid needless
// `gh` invocations); actual identity enforcement comes entirely from
// `gh attestation verify --owner GITHUB_ORG`, backed by the cryptographic
// certificate chain and Rekor transparency log, not from that pre-filter.
//
// Deployments with more than one container are not verified because the
// repository cannot be attributed to a single image unambiguously.
//
// TODO: Link deployments with more than one container to source
// repositories, verifying each container image independently.
//
// TODO: Implement support for private OCI registries other than ghcr.io.
// `gh attestation verify oci://...` requires the environment to already be
// authenticated with the artifact's container registry; ghcr.io works out
// of the box using GITHUB_TOKEN, other registries currently do not.
//
// # Environment Variables
//
//   - GITHUB_ORG (mandatory) - limits collection to repositories whose
//     attestations are verified for this GitHub organization, via
//     `gh attestation verify --owner`.
//   - GITHUB_TOKEN (optional) - GitHub API bearer token used both for the
//     REST API calls (repository metadata, CODEOWNERS) and, as GH_TOKEN,
//     for `gh attestation verify`.
//   - GITHUB_BASE_URL (optional) - GitHub API base URL; defaults to
//     "https://api.github.com". Set this for GitHub Enterprise.
//   - GITHUB_HTTP_TIMEOUT (optional) - GitHub API request timeout; defaults
//     to 10s.
//   - GITHUB_ATTESTATION_TIMEOUT (optional) - timeout for a single
//     `gh attestation verify` invocation; defaults to 30s.
//   - GH_CLI_PATH (optional) - path to the `gh` binary; defaults to "gh"
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
	"time"

	"github.com/naira-project/naira/plugins/internal/deploymentdiscovery"
	"github.com/naira-project/naira/plugins/internal/kubeutil"
	"github.com/naira-project/naira/plugins/internal/repositoryidentity"
	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/naira-project/naira/plugins/pkg/pluginmain"
	"k8s.io/client-go/kubernetes"
)

const (
	propertyKeyURL      = "url"
	propertyKeyLanguage = "language"
	propertyKeyHomepage = "homepage"
)

type config struct {
	// GitHubOrg limits collection to repositories whose attestations are
	// verified for this GitHub organization.
	GitHubOrg string `env:"GITHUB_ORG"`

	Kubeconfig string `env:"KUBECONFIG"`

	GitHubToken   string        `env:"GITHUB_TOKEN"`
	GitHubBaseURL string        `env:"GITHUB_BASE_URL" default:"https://api.github.com"`
	HTTPTimeout   time.Duration `env:"GITHUB_HTTP_TIMEOUT" default:"10s"`

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
		attestation: newAttestationVerifier(config.GHCLIPath, config.GitHubToken, config.GitHubBaseURL, config.AttestationTimeout),
		logger:      logger,
		config:      config,
	}
}

func main() {
	app := pluginmain.New[config]()
	if app.PluginConfig.GitHubOrg == "" {
		app.Logger.Fatal("GITHUB_ORG is required")
	}

	app.Serve(New(app.PluginConfig, app.Logger))
}

func (p *Plugin) Collect(ctx context.Context) (pluginapi.CollectResponse, error) {
	k8sClient, err := p.connect()
	if err != nil {
		return pluginapi.CollectResponse{}, fmt.Errorf("connecting to cluster: %w", err)
	}
	return p.collect(ctx, k8sClient)
}

func (p *Plugin) collect(ctx context.Context, k8sClient kubernetes.Interface) (pluginapi.CollectResponse, error) {
	entries, err := deploymentdiscovery.DiscoverDeployments(ctx, k8sClient, p.logger)
	if err != nil {
		return pluginapi.CollectResponse{}, fmt.Errorf("discovering deployments: %w", err)
	}

	var resp pluginapi.CollectResponse
	repoDone := make(map[string]bool) // repo node path -> repo already collected

	for _, entry := range entries {
		if len(entry.Images) != 1 {
			// Ambiguous attribution with more than one container - see the
			// package doc TODO.
			continue
		}

		image := entry.Images[0]
		if !imageReferencesOrg(image, p.config.GitHubOrg) {
			continue
		}

		owner, name, verified, err := p.attestation.Verify(ctx, image, p.config.GitHubOrg)
		if err != nil {
			p.logger.Printf("github plugin: verifying attestation for %s (deployment %s/%s): %v", entry.Images[0], entry.Namespace, entry.Name, err)
			continue
		}
		if !verified {
			continue
		}

		repoNodeID := gitRepositoryNodeID(owner, name)

		if !repoDone[repoNodeID.Path] {
			nodes, relations, err := p.collectRepo(ctx, owner, name)
			if err != nil {
				// One bad repo (renamed, deleted, no access) shouldn't take
				// down the whole snapshot - log and move on.
				p.logger.Printf("github plugin: skipping %s/%s: %v", owner, name, err)
				continue
			}
			resp.Nodes = append(resp.Nodes, nodes...)
			resp.Relations = append(resp.Relations, relations...)
			repoDone[repoNodeID.Path] = true
		}

		resp.Nodes = append(resp.Nodes, pluginapi.NodeClaim{
			ID: entry.NodeID(),
		})

		resp.Relations = append(resp.Relations, pluginapi.RelationClaim{
			Kind: pluginapi.RelationKindBuiltFrom,
			From: entry.NodeID(),
			To:   repoNodeID,
		})
	}

	return resp, nil
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
	var relations []pluginapi.RelationClaim

	codeowners, found, err := p.github.GetCodeowners(ctx, owner, name)
	if err != nil {
		return nil, nil, fmt.Errorf("fetching codeowners: %w", err)
	}
	if found {
		for _, owner := range parseCodeowners(codeowners) {
			ownerNodeID := pluginapi.NodeID{Kind: pluginapi.NodeKindOwner, Path: owner}
			nodes = append(nodes, pluginapi.NodeClaim{ID: ownerNodeID})
			relations = append(relations, pluginapi.RelationClaim{
				Kind: pluginapi.RelationKindOwnedBy,
				From: repoNodeID,
				To:   ownerNodeID,
			})
		}
	}

	return nodes, relations, nil
}

func gitRepositoryNodeID(owner, name string) pluginapi.NodeID {
	return pluginapi.NodeID{
		Kind: pluginapi.NodeKindGitRepository,
		Path: repositoryidentity.GitHubRepositoryNodePath(owner, name),
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
