package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	thalamusv1alpha1 "github.com/cobaltcore-dev/thalamus/api/v1alpha1"
	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/naira-project/naira/plugins/pkg/pluginmain"
)

const (
	pluginName = "thalamus"

	propertyKeyOwnedBy         = "owned_by"
	propertyKeyNamespace       = "namespace"
	propertyKeyPhase           = "phase"
	propertyKeyEngineType      = "engine_type"
	propertyKeyEPPType         = "epp_type"
	propertyKeyEngineImage     = "engine_image"
	propertyKeyEngineArgs      = "engine_args"
	propertyKeyWeightsType     = "weights_type"
	propertyKeyWeightsHFRepoID = "weights_hf_repo_id"
	propertyKeyHasEPP          = "has_epp"
	propertyKeyEPPImage        = "epp_image"
	propertyKeyNodeSelector    = "node_selector"
)

type config struct {
	BaseURL     string        `env:"THALAMUS_BASE_URL" default:"http://127.0.0.1:5000"`
	BearerToken string        `env:"THALAMUS_BEARER_TOKEN"`
	HTTPTimeout time.Duration `env:"THALAMUS_HTTP_TIMEOUT" default:"5s"`
}

type Plugin struct {
	httpClient    *http.Client
	logger        *log.Logger
	config        config
	modelProvider ModelProvider
}

func New(config config, logger *log.Logger) *Plugin {
	return &Plugin{
		httpClient:    &http.Client{Timeout: config.HTTPTimeout},
		logger:        logger,
		config:        config,
		modelProvider: newModelProvider(logger),
	}
}

func main() {
	app := pluginmain.New[config]()
	p := New(app.PluginConfig, app.Logger)
	app.Serve(p)
}

func (p *Plugin) Collect(ctx context.Context) (pluginapi.CollectResponse, error) {
	if p.modelProvider != nil {
		return p.collectFromCRD(ctx)
	}
	return p.collectFromAPI(ctx)
}

func (p *Plugin) collectFromCRD(ctx context.Context) (pluginapi.CollectResponse, error) {
	crdModels, err := p.modelProvider.ListModels(ctx)
	if err != nil {
		return pluginapi.CollectResponse{}, fmt.Errorf("listing thalamus CRD models: %w", err)
	}

	nodes := make([]pluginapi.NodeClaim, 0, len(crdModels)*2)
	relations := make([]pluginapi.RelationClaim, 0, len(crdModels))

	for _, crd := range crdModels {
		modelNode := pluginapi.NodeClaim{
			ID: pluginapi.NodeID{
				Kind: pluginapi.NodeKindModel,
				Path: pluginName + "/" + crd.Namespace + "/" + crd.Name,
			},
			Properties: crdProperties(crd),
		}
		nodes = append(nodes, modelNode)

		if crd.Spec.Weights.HF != nil {
			repoNode := pluginapi.NodeClaim{
				ID: pluginapi.NodeID{
					Kind: pluginapi.NodeKindGitRepository,
					Path: "huggingface/" + crd.Spec.Weights.HF.RepoID,
				},
				Properties: pluginapi.PropertyMap{},
			}
			nodes = append(nodes, repoNode)
			relations = append(relations, pluginapi.RelationClaim{
				Kind: pluginapi.RelationKindSourcedFrom,
				From: modelNode.ID,
				To:   repoNode.ID,
			})
		}
	}

	return pluginapi.CollectResponse{Nodes: nodes, Relations: relations}, nil
}

func (p *Plugin) collectFromAPI(ctx context.Context) (pluginapi.CollectResponse, error) {
	models, err := p.fetchModels(ctx)
	if err != nil {
		return pluginapi.CollectResponse{}, fmt.Errorf("fetching thalamus models: %w", err)
	}

	nodes := make([]pluginapi.NodeClaim, 0, len(models))
	for _, m := range models {
		nodes = append(nodes, pluginapi.NodeClaim{
			ID: pluginapi.NodeID{
				Kind: pluginapi.NodeKindModel,
				Path: pluginName + "/" + m.ID,
			},
			Properties: pluginapi.PropertyMap{
				propertyKeyOwnedBy: m.OwnedBy,
			},
		})
	}

	return pluginapi.CollectResponse{Nodes: nodes, Relations: []pluginapi.RelationClaim{}}, nil
}

func crdProperties(crd thalamusv1alpha1.Model) pluginapi.PropertyMap {
	props := pluginapi.PropertyMap{
		propertyKeyNamespace:   crd.Namespace,
		propertyKeyPhase:       string(crd.Status.Phase),
		propertyKeyEngineType:  string(crd.Status.EngineType),
		propertyKeyEPPType:     string(crd.Status.EPPType),
		propertyKeyEngineImage: crd.Spec.Serving.Engine.Image,
		propertyKeyWeightsType: string(crd.Spec.Weights.Type),
	}

	if crd.Spec.Weights.HF != nil {
		props[propertyKeyWeightsHFRepoID] = crd.Spec.Weights.HF.RepoID
	}

	if len(crd.Spec.Serving.Engine.Args) > 0 {
		if b, err := json.Marshal(crd.Spec.Serving.Engine.Args); err == nil {
			props[propertyKeyEngineArgs] = string(b)
		}
	}

	if crd.Spec.Serving.EPP != nil {
		props[propertyKeyHasEPP] = "true"
		props[propertyKeyEPPImage] = crd.Spec.Serving.EPP.Image
	} else {
		props[propertyKeyHasEPP] = "false"
	}

	if crd.Spec.Scheduling != nil && len(crd.Spec.Scheduling.NodeSelector) > 0 {
		if b, err := json.Marshal(crd.Spec.Scheduling.NodeSelector); err == nil {
			props[propertyKeyNodeSelector] = string(b)
		}
	}

	return props
}

func (p *Plugin) fetchModels(ctx context.Context) ([]model, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.config.BaseURL+"/v1/models", nil)
	if err != nil {
		return nil, fmt.Errorf("building thalamus models request: %w", err)
	}
	p.addAuthorization(req)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling thalamus models endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("thalamus models endpoint returned %s", resp.Status)
	}

	var payload modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding thalamus models response: %w", err)
	}

	return payload.Data, nil
}

func (p *Plugin) addAuthorization(req *http.Request) {
	if strings.TrimSpace(p.config.BearerToken) != "" {
		req.Header.Set("Authorization", "Bearer "+p.config.BearerToken)
	}
}

type modelsResponse struct {
	Object string  `json:"object"`
	Data   []model `json:"data"`
}

type model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}
