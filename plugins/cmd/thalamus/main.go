// thalamus plugin discovers models served by Thalamus
// (https://github.com/cobaltcore-dev/thalamus), a Kubernetes-native LLM
// serving operator.
//
// When running in-cluster, models are collected from thalamus.cloud/v1alpha1
// Model custom resources via the Kubernetes API. Each Model CR becomes a
// model Node (its path uses the Hugging Face repo ID when the weights are
// sourced from Hugging Face, falling back to the CR name otherwise), with
// properties describing its namespace, phase, engine, EPP and scheduling
// configuration. A Model whose weights are sourced from Hugging Face also
// gets a git_repository Node for that Hugging Face repo, linked to the model
// via a "sourced_from" relation.
//
// When not running in-cluster (e.g. during local development), the plugin
// instead queries THALAMUS_BASE_URL's OpenAI API-compatible "/v1/models"
// endpoint directly, emitting a model Node per returned model id with only
// its "owned_by" property.
//
// # Related Plugins
//
// This plugin is a part of a family of Thalamus-related plugins, including:
//
//   - thalamus - discovers Thalamus models and their properties from the
//     Thalamus CRDs or OpenAI API-compatible endpoint.
//   - depl_uses_thalamus - discovers which Deployments consume the Thalamus
//     gateway, linking them to the models they use.
//
// # Known Issues
//
// This plugin is third-party and HIGHLY EXPERIMENTAL. Notably, the Thalamus
// custom resources were NOT SUPPORTED BY THALAMUS yet at the time of writing.
//
// # Environment Variables
//
//   - THALAMUS_BASE_URL (optional) - base URL of the Thalamus OpenAI
//     API-compatible endpoint, used only when the in-cluster CRD source is
//     unavailable; defaults to: "http://127.0.0.1:5000".
//   - THALAMUS_BEARER_TOKEN (optional) - bearer token sent to
//     THALAMUS_BASE_URL; if unset, the request is made unauthenticated.
//   - HTTP_TIMEOUT (optional) - HTTP request timeout;
//     defaults to: 5s.
//   - PATH_PREFIX (optional) - prefix for the emitted model Node paths, e.g.
//     "thalamus" yields "thalamus/gpt-oss-120b"; defaults to: "thalamus".
//
//go:generate bash -c "goreadme -use-stdlib-markdown -title 'thalamus plugin' | sed 's/ {#hdr-[^}]*}//g' > README.md"
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	thalamusv1alpha1 "github.com/cobaltcore-dev/thalamus/api/v1alpha1"
	"github.com/naira-project/naira/plugins/internal/openaicompat"
	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/naira-project/naira/plugins/pkg/pluginmain"
)

const (
	propertyKeyOwnedBy         = "owned_by"
	propertyKeyNamespace       = "namespace"
	propertyKeyName            = "name"
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
	PathPrefix  string        `env:"PATH_PREFIX" default:"thalamus" usage:"prefix for emitted model Node paths, e.g. 'thalamus' yields 'thalamus/gpt-oss-120b'"`
	BaseURL     string        `env:"THALAMUS_BASE_URL" default:"http://127.0.0.1:5000"`
	BearerToken string        `env:"THALAMUS_BEARER_TOKEN"`
	HTTPTimeout time.Duration `env:"HTTP_TIMEOUT" default:"5s"`
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
		modelPath := crd.Name
		if crd.Spec.Weights.HF != nil {
			modelPath = crd.Spec.Weights.HF.RepoID
		}
		modelNode := pluginapi.NodeClaim{
			ID: pluginapi.NodeID{
				Kind: pluginapi.NodeKindModel,
				Path: p.config.PathPrefix + "/" + modelPath,
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
	var models openaicompat.SimpleModelsResponse
	err := openaicompat.GetModels(ctx, p.httpClient, p.config.BaseURL, p.config.BearerToken, &models)
	if err != nil {
		return pluginapi.CollectResponse{}, fmt.Errorf("fetching thalamus models: %w", err)
	}

	nodes := make([]pluginapi.NodeClaim, 0, len(models.Data))
	for _, m := range models.Data {
		nodes = append(nodes, pluginapi.NodeClaim{
			ID: pluginapi.NodeID{
				Kind: pluginapi.NodeKindModel,
				Path: p.config.PathPrefix + "/" + m.ID,
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
		propertyKeyName:        crd.Name,
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
