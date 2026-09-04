// github enriches GitHub repositories discovered from Kubernetes Deployments
// with repository metadata and CODEOWNERS ownership information.
//
// The plugin only collects repositories that are both referenced by a
// Kubernetes Deployment and owned by the GitHub organization configured in
// GITHUB_ORG. It does not enumerate or collect repositories outside that
// organization. The organization restriction is applied using the GITHUB_ORG
// environment variable.
//
// TODO: Implement support for private OCI registries. Source repository
// discovery may fail for images stored in registries requiring
// authentication.
//
// # Environment Variables
//
//   - GITHUB_ORG (mandatory) - limits collection to repositories owned by this
//     GitHub organization.
//   - GITHUB_TOKEN (optional) - GitHub API bearer token used to access the
//     repositories and CODEOWNERS files.
//   - GITHUB_BASE_URL (optional) - GitHub API base URL; defaults to
//     "https://api.github.com". Set this for GitHub Enterprise.
//   - GITHUB_HTTP_TIMEOUT (optional) - GitHub API request timeout; defaults to
//     10s.
//   - KUBECONFIG (optional) - path to a kubeconfig file; when unset, in-cluster
//     configuration is used.
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

	"github.com/naira-project/naira/plugins/internal/deploymentdiscovery"
	"github.com/naira-project/naira/plugins/internal/kubeutil"
	"github.com/naira-project/naira/plugins/internal/repositoryidentity"
	"github.com/naira-project/naira/plugins/internal/sourcerepository"
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
	// GitHubOrg limits collection to repositories used by Deployments and owned
	// by this GitHub organization.
	GitHubOrg string `env:"GITHUB_ORG"`

	Kubeconfig string `env:"KUBECONFIG"`

	GitHubToken   string        `env:"GITHUB_TOKEN"`
	GitHubBaseURL string        `env:"GITHUB_BASE_URL" default:"https://api.github.com"`
	HTTPTimeout   time.Duration `env:"GITHUB_HTTP_TIMEOUT" default:"10s"`
}

type Plugin struct {
	github *githubClient
	logger *log.Logger
	config config
}

func New(config config, logger *log.Logger) *Plugin {
	return &Plugin{
		github: newGithubClient(&http.Client{Timeout: config.HTTPTimeout}, config.GitHubBaseURL, config.GitHubToken),
		logger: logger,
		config: config,
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
	repos, err := p.resolveRepos(ctx, k8sClient)
	if err != nil {
		return pluginapi.CollectResponse{}, fmt.Errorf("resolving repos to collect: %w", err)
	}

	var resp pluginapi.CollectResponse

	for _, ref := range repos {
		nodes, relations, err := p.collectRepo(ctx, ref.owner, ref.name)
		if err != nil {
			// One bad repo (renamed, deleted, no access) shouldn't take down
			// the whole snapshot — log and move on.
			p.logger.Printf("git plugin: skipping %s/%s: %v", ref.owner, ref.name, err)
			continue
		}
		resp.Nodes = append(resp.Nodes, nodes...)
		resp.Relations = append(resp.Relations, relations...)
	}

	return resp, nil
}

type repoRef struct {
	owner string
	name  string
}

// resolveRepos figures out which GitHub repos to collect by running repository discovery
// and filtering by GitHub host and p.config.GitHubOrg if set.
func (p *Plugin) resolveRepos(ctx context.Context, k8sClient kubernetes.Interface) ([]repoRef, error) {
	discovered, err := discoverRepos(ctx, k8sClient, p.logger)
	if err != nil {
		return nil, fmt.Errorf("discovering repositories from deployments: %w", err)
	}

	var refs []repoRef

	for _, repo := range discovered {
		// Only collect GitHub repositories with valid owner/name parsed
		owner, name, ok := repositoryidentity.ParseGitHubRepository(repo.URL)
		if !ok {
			continue
		}
		if owner == "" || name == "" {
			continue
		}

		if p.config.GitHubOrg != "" && !strings.EqualFold(owner, p.config.GitHubOrg) {
			continue
		}

		refs = append(refs, repoRef{
			owner: owner,
			name:  name,
		})
	}

	return refs, nil
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

// discoverRepos returns unique source repositories found in Deployments.
func discoverRepos(ctx context.Context, client kubernetes.Interface, logger *log.Logger) ([]sourcerepository.Repository, error) {
	entries, err := deploymentdiscovery.DiscoverDeployments(ctx, client, logger)
	if err != nil {
		return nil, fmt.Errorf("discovering deployment repositories: %w", err)
	}
	seen := map[string]bool{}
	result := make([]sourcerepository.Repository, 0, len(entries))
	for _, entry := range entries {
		path := repositoryidentity.GitHubRepositoryNodePathFromURL(entry.SourceRepository.URL)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		result = append(result, entry.SourceRepository)
	}
	return result, nil
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
