package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/naira-project/naira/plugins/internal/openaiutil"
	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/naira-project/naira/plugins/pkg/pluginmain"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

const (
	propertyKeyDiscoveredVia     = "discovered_via"
	propertyKeyOwnedBy           = "owned_by"
	propertyKeyLiteLLMVirtualKey = "litellm_virtual_key"
	propertyKeyNamespace         = "namespace"
	propertyKeyTeam              = "team"

	propertyValueKeyInfo = "key_info"
)

type config struct {
	PathPrefix  string        `env:"PATH_PREFIX" default:"litellm"`
	BaseURL     string        `env:"LITELLM_BASE_URL" default:"http://127.0.0.1:4000"`
	APIKey      string        `env:"LITELLM_API_KEY"`
	HTTPTimeout time.Duration `env:"LITELLM_HTTP_TIMEOUT" default:"5s"`
}

type Plugin struct {
	httpClient          *http.Client
	logger              *log.Logger
	config              config
	appIdentityProvider AppIdentityProvider
}

func New(config config, logger *log.Logger) *Plugin {
	return &Plugin{
		httpClient:          &http.Client{Timeout: config.HTTPTimeout},
		logger:              logger,
		config:              config,
		appIdentityProvider: newAppIdentityProvider(logger),
	}
}

func main() {
	app := pluginmain.New[config]()

	p := New(app.PluginConfig, app.Logger)

	app.Serve(p)
}

func (p *Plugin) Collect(ctx context.Context) (pluginapi.CollectResponse, error) {
	// List models from openai compatible v1/models endpoint
	models, err := openaiutil.FetchModels(ctx, p.httpClient, p.config.BaseURL, p.config.APIKey)
	if err != nil {
		return pluginapi.CollectResponse{}, fmt.Errorf("fetching LiteLLM models: %w", err)
	}

	nodes := make([]pluginapi.NodeClaim, 0, len(models))
	relations := make([]pluginapi.RelationClaim, 0)
	modelKeys := make(map[string]pluginapi.NodeClaim, len(models))
	seenRelations := make(map[string]struct{})
	fetchAllowedModelsErrors := make([]error, 0)

	for _, model := range models {
		node := pluginapi.NodeClaim{
			ID: pluginapi.NodeID{Kind: pluginapi.NodeKindModel, Path: p.config.PathPrefix + "/" + model.ID},
			Properties: pluginapi.PropertyMap{
				propertyKeyOwnedBy: model.OwnedBy,
			},
		}
		nodes = append(nodes, node)
		modelKeys[model.ID] = node
	}

	if p.appIdentityProvider == nil {
		return pluginapi.CollectResponse{Nodes: nodes, Relations: relations}, nil
	}

	apps, err := p.appIdentityProvider.ListAppIdentities(ctx)
	if err != nil {
		if p.logger != nil {
			p.logger.Printf("listing LiteLLM app identities failed, continuing without app identities: %v", err)
		}
		return pluginapi.CollectResponse{Nodes: nodes, Relations: relations}, nil
	}

	for _, app := range apps {
		appNode := pluginapi.NodeClaim{
			ID: applicationNodeID(app, p.config.PathPrefix),
			Properties: pluginapi.PropertyMap{
				propertyKeyNamespace:         app.Namespace,
				propertyKeyTeam:              app.Team,
				propertyKeyLiteLLMVirtualKey: app.LiteLLMVirtualKey,
			},
		}
		nodes = append(nodes, appNode)

		if strings.TrimSpace(app.LiteLLMVirtualKey) == "" {
			continue
		}

		allowedModels, err := p.fetchAllowedModels(ctx, app.LiteLLMVirtualKey)
		if err != nil {
			fetchAllowedModelsErrors = append(fetchAllowedModelsErrors, fmt.Errorf("fetching allowed LiteLLM models for app %q: %w", app.Name, err))
			continue
		}

		for _, modelName := range allowedModels {
			modelName = strings.TrimSpace(modelName)
			if modelName == "" {
				continue
			}

			if _, ok := modelKeys[modelName]; !ok {
				node := pluginapi.NodeClaim{
					ID: pluginapi.NodeID{Kind: pluginapi.NodeKindModel, Path: p.config.PathPrefix + "/" + modelName},
					Properties: pluginapi.PropertyMap{
						propertyKeyDiscoveredVia: propertyValueKeyInfo,
					},
				}
				nodes = append(nodes, node)
				modelKeys[modelName] = node
			}

			relation := pluginapi.RelationClaim{
				Kind: pluginapi.RelationKindUsesModel,
				From: appNode.ID,
				To:   modelKeys[modelName].ID,
				Properties: pluginapi.PropertyMap{
					propertyKeyLiteLLMVirtualKey: app.LiteLLMVirtualKey,
				},
			}
			relationKey := relation.From.Kind + ":" + relation.From.Path + "|" + relation.To.Kind + ":" + relation.To.Path + "|" + relation.Kind
			if _, seen := seenRelations[relationKey]; seen {
				continue
			}
			seenRelations[relationKey] = struct{}{}
			relations = append(relations, relation)
		}
	}

	response := pluginapi.CollectResponse{Nodes: dedupeNodes(nodes), Relations: relations}
	if len(fetchAllowedModelsErrors) > 0 {
		return response, errors.Join(fetchAllowedModelsErrors...)
	}

	return response, nil
}

func (p *Plugin) fetchAllowedModels(ctx context.Context, key string) ([]string, error) {
	// Extract littellm specific model information from /key/info endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.config.BaseURL+"/key/info", nil)
	if err != nil {
		return nil, fmt.Errorf("building LiteLLM key info request: %w", err)
	}

	query := req.URL.Query()
	query.Set("key", key)
	req.URL.RawQuery = query.Encode()
	p.addAuthorization(req)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling LiteLLM key info endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("litellm /key/info returned %s", resp.Status)
	}

	var payload keyInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding LiteLLM key info response: %w", err)
	}

	return payload.Info.Models, nil
}

func (p *Plugin) addAuthorization(req *http.Request) {
	if strings.TrimSpace(p.config.APIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	}
}

func newAppIdentityProvider(logger *log.Logger) AppIdentityProvider {
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		logger.Printf("not running in cluster, k8s sources disabled: %v", err)
		return nil
	}

	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		logger.Printf("failed to create dynamic k8s client: %v", err)
		return nil
	}

	return NewKubernetesAppIdentityProvider(dynamicClient)
}

func applicationNodeID(app AppIdentity, pathPrefix string) pluginapi.NodeID {
	if strings.TrimSpace(app.Namespace) == "" {
		return pluginapi.NodeID{Kind: pluginapi.NodeKindApplication, Path: pathPrefix + "/" + app.Name}
	}

	return pluginapi.NodeID{Kind: pluginapi.NodeKindApplication, Path: pathPrefix + "/" + app.Namespace + "/" + app.Name}
}

func dedupeNodes(nodes []pluginapi.NodeClaim) []pluginapi.NodeClaim {
	unique := make(map[string]pluginapi.NodeClaim, len(nodes))
	for _, node := range nodes {
		unique[node.ID.Kind+":"+node.ID.Path] = node
	}

	result := make([]pluginapi.NodeClaim, 0, len(unique))
	for _, node := range unique {
		result = append(result, node)
	}

	return result
}

type keyInfoResponse struct {
	Info struct {
		Models []string `json:"models"`
	} `json:"info"`
}
