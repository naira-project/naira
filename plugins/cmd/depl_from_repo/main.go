// depl_from_repo scans Kubernetes Deployments and links them to the Git
// repositories from which they were deployed.
//
// For every discovered Deployment, the plugin emits a Deployment node with
// container-image properties. When repository metadata can be
// extracted from the Deployment, it emits a GitRepository node and a
// deployed_from relation from the Deployment to that repository.
//
// TODO: Implement support for private OCI registries. Source repository
// discovery may fail for images stored in registries requiring
// authentication.
//
// # Environment Variables
//
//   - KUBECONFIG (optional) - path to a kubeconfig file; when unset, in-cluster
//     configuration is used.
//
//go:generate bash -c "goreadme -use-stdlib-markdown -title 'depl_from_repo plugin' | sed 's/ {#hdr-[^}]*}//g' > README.md"
package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"k8s.io/client-go/kubernetes"

	"github.com/naira-project/naira/plugins/internal/deploymentdiscovery"
	"github.com/naira-project/naira/plugins/internal/kubeutil"
	"github.com/naira-project/naira/plugins/internal/repositoryidentity"
	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/naira-project/naira/plugins/pkg/pluginmain"
)

const (
	propertyKeyNamespace = "namespace"
	propertyKeyImages    = "images"
	propertyKeyRepoURL   = "url"
	propertyKeyDetection = "detection_method"
)

type config struct {
	Kubeconfig string `env:"KUBECONFIG"`
}

type Plugin struct {
	k8sClient *kubernetes.Clientset
	logger    *log.Logger
	config    config
}

func New(cfg config, logger *log.Logger) *Plugin {
	return &Plugin{config: cfg, logger: logger}
}

func main() {
	app := pluginmain.New[config]()

	app.Serve(New(app.PluginConfig, app.Logger))
}

func (p *Plugin) Collect(ctx context.Context) (pluginapi.CollectResponse, error) {
	k8sClient, err := p.connect()
	if err != nil {
		return pluginapi.CollectResponse{}, fmt.Errorf("connecting to cluster: %w", err)
	}
	return p.collect(ctx, k8sClient)
}

func (p *Plugin) collect(ctx context.Context, k8sClient *kubernetes.Clientset) (pluginapi.CollectResponse, error) {
	var nodes []pluginapi.NodeClaim
	var relations []pluginapi.RelationClaim

	entries, err := deploymentdiscovery.DiscoverDeployments(ctx, k8sClient, p.logger)
	if err != nil {
		return pluginapi.CollectResponse{}, fmt.Errorf("discovering deployments: %w", err)
	}

	seenRepos := make(map[string]bool)

	for _, entry := range entries {
		depPath := fmt.Sprintf("%s/%s/%s", entry.ClusterID, entry.Namespace, entry.Name)
		depNodeID := pluginapi.NodeID{
			Kind: pluginapi.NodeKindDeployment,
			Path: depPath,
		}

		// Always emit the deployment node with its images.
		depProps := pluginapi.PropertyMap{
			propertyKeyNamespace: entry.Namespace,
			propertyKeyImages:    strings.Join(entry.Images, ", "),
		}
		nodes = append(nodes, pluginapi.NodeClaim{
			ID:         depNodeID,
			Properties: depProps,
		})

		repoPath := repositoryidentity.GitHubRepositoryNodePathFromURL(entry.SourceRepository.URL)
		// Repository linkage is optional — only create the git node + relation
		// when a repository was successfully discovered for this deployment.
		if repoPath == "" {
			continue
		}

		gitNodeID := pluginapi.NodeID{
			Kind: pluginapi.NodeKindGitRepository,
			Path: repoPath,
		}

		if !seenRepos[repoPath] {
			nodes = append(nodes, pluginapi.NodeClaim{
				ID: gitNodeID,
				Properties: pluginapi.PropertyMap{
					propertyKeyRepoURL:   entry.SourceRepository.URL,
					propertyKeyDetection: entry.SourceRepository.Method,
				},
			})
			seenRepos[repoPath] = true
		}

		relations = append(relations, pluginapi.RelationClaim{
			Kind: pluginapi.RelationKindDeployedFrom,
			From: depNodeID,
			To:   gitNodeID,
		})
	}

	return pluginapi.CollectResponse{
		Nodes:     nodes,
		Relations: relations,
	}, nil
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
