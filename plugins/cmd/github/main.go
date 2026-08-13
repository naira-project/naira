package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/naira-project/naira/plugins/internal/kubeutil"
	"github.com/naira-project/naira/plugins/internal/repositorydiscovery"
	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/naira-project/naira/plugins/pkg/pluginmain"
	"k8s.io/client-go/kubernetes"
)

const (
	propertyKeyDescription   = "description"
	propertyKeyURL           = "url"
	propertyKeyDefaultBranch = "default_branch"
	propertyKeyLanguage      = "language"
	propertyKeyArchived      = "archived"
	propertyKeyFork          = "fork"
	propertyKeyTopics        = "topics"
	propertyKeyStars         = "stars"
	propertyKeyHomepage      = "homepage"
	propertyKeyLastCommitAt  = "last_commit_at"
	propertyKeyLastReleaseAt = "last_release_at"
	propertyKeyLastReleaseID = "last_release"
)

type config struct {
	PathPrefix string `env:"PATH_PREFIX" default:"git"`

	// GitHubOrg limits collection to repositories used by Deployments and owned
	// by this GitHub organization. It no longer means "list every repo".
	GitHubOrg string `env:"GITHUB_ORG" default:"naira-project"`

	Kubeconfig string `env:"KUBECONFIG"`

	GitHubToken   string        `env:"GITHUB_TOKEN" default:""`
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
	discovered, err := repositorydiscovery.Discover(ctx, k8sClient, p.logger)
	if err != nil {
		return nil, fmt.Errorf("discovering repositories from deployments: %w", err)
	}

	var refs []repoRef

	for _, repo := range discovered {
		// Only collect GitHub repositories with valid owner/name parsed
		if repo.Owner == "" || repo.Name == "" {
			continue
		}

		if p.config.GitHubOrg != "" && !strings.EqualFold(repo.Owner, p.config.GitHubOrg) {
			continue
		}

		refs = append(refs, repoRef{
			owner: repo.Owner,
			name:  repo.Name,
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
		propertyKeyURL:           repo.HTMLURL,
		propertyKeyDefaultBranch: repo.DefaultBranch,
		propertyKeyArchived:      fmt.Sprintf("%t", repo.Archived),
		propertyKeyFork:          fmt.Sprintf("%t", repo.Fork),
	}
	if repo.Description != "" {
		props[propertyKeyDescription] = repo.Description
	}
	if repo.Language != "" {
		props[propertyKeyLanguage] = repo.Language
	}
	if repo.Homepage != "" {
		props[propertyKeyHomepage] = repo.Homepage
	}
	if len(repo.Topics) > 0 {
		props[propertyKeyTopics] = strings.Join(repo.Topics, ",")
	}
	props[propertyKeyStars] = fmt.Sprintf("%d", repo.StargazersCount)

	if lastCommitAt, found, err := p.github.GetLatestCommitDate(ctx, owner, name); err != nil {
		return nil, nil, fmt.Errorf("fetching latest commit: %w", err)
	} else if found {
		props[propertyKeyLastCommitAt] = lastCommitAt
	}

	if release, found, err := p.github.GetLatestRelease(ctx, owner, name); err != nil {
		return nil, nil, fmt.Errorf("fetching latest release: %w", err)
	} else if found {
		props[propertyKeyLastReleaseID] = release.TagName
		props[propertyKeyLastReleaseAt] = release.PublishedAt
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

// gitRepositoryNodeID builds a cluster-independent NodeID for a GitHub repository.
func gitRepositoryNodeID(owner, name string) pluginapi.NodeID {
	return pluginapi.NodeID{
		Kind: pluginapi.NodeKindGitRepository,
		Path: fmt.Sprintf("github.com/%s/%s", owner, name),
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
