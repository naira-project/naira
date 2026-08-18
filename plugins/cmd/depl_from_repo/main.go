package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"k8s.io/client-go/kubernetes"

	"github.com/naira-project/naira/plugins/internal/kubeutil"
	"github.com/naira-project/naira/plugins/internal/repositorydiscovery"
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
	Kubeconfig  string        `env:"KUBECONFIG"`
	HTTPTimeout time.Duration `env:"HTTP_TIMEOUT" default:"10s"`
}

type Plugin struct {
	k8sClient *kubernetes.Clientset
	logger    *log.Logger
	config    config
}

func New(cfg config, logger *log.Logger) (*Plugin, error) {
	return &Plugin{config: cfg, logger: logger}, nil
}

func main() {
	app := pluginmain.New[config]()

	p, err := New(app.PluginConfig, app.Logger)
	if err != nil {
		app.Logger.Fatalf("Plugin initialization error: %v", err)
	}

	app.Serve(p)
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

	entries, err := repositorydiscovery.DiscoverDeployments(ctx, k8sClient, p.logger)
	if err != nil {
		return pluginapi.CollectResponse{}, fmt.Errorf("discovering deployments: %w", err)
	}

	seenRepos := make(map[string]bool)

	for _, entry := range entries {
		depPath := fmt.Sprintf("%s/%s/%s", entry.ClusterID, entry.Namespace, entry.Deployment)
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

		// Repository linkage is optional — only create the git node + relation
		// when a repository was successfully discovered for this deployment.
		if entry.Repository.URL == "" {
			continue
		}

		repoPath := repositorydiscovery.CanonicalPathFromRepo(entry.Repository)
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
					propertyKeyRepoURL:   entry.Repository.URL,
					propertyKeyDetection: entry.Repository.Method,
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
