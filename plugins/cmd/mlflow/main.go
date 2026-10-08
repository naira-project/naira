package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/naira-project/naira/plugins/pkg/httpjson"
	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/naira-project/naira/plugins/pkg/pluginmain"
)

const (
	propertyKeyDigest      = "digest"
	propertyKeyRunID       = "run_id"
	propertyKeySourceType  = "source_type"
	propertyKeyDescription = "description"
)

type config struct {
	PathPrefix  string        `env:"PATH_PREFIX" default:"mlflow"`
	BaseURL     string        `env:"MLFLOW_BASE_URL" default:"http://127.0.0.1:5000"`
	BearerToken string        `env:"MLFLOW_BEARER_TOKEN"`
	HTTPTimeout time.Duration `env:"MLFLOW_HTTP_TIMEOUT" default:"5s"`
}

type Plugin struct {
	httpClient *http.Client
	config     config
}

func New(config config) *Plugin {
	return &Plugin{
		httpClient: &http.Client{Timeout: config.HTTPTimeout},
		config:     config,
	}
}

func main() {
	app := pluginmain.New[config]()
	p := New(app.PluginConfig)
	app.Serve(p)
}

func (p *Plugin) Collect(ctx context.Context) (pluginapi.CollectResponse, error) {
	registeredModels, err := p.fetchRegisteredModels(ctx)
	if err != nil {
		return pluginapi.CollectResponse{}, fmt.Errorf("fetching MLflow registered models: %w", err)
	}

	nodes := make([]pluginapi.NodeClaim, 0, len(registeredModels))
	relations := make([]pluginapi.RelationClaim, 0)
	seenDatasets := map[string]pluginapi.NodeClaim{}
	fetchRunErrors := make([]error, 0)

	for _, item := range registeredModels {
		modelNode := pluginapi.NodeClaim{
			ID: pluginapi.NodeID{Kind: pluginapi.NodeKindModel, Path: p.config.PathPrefix + "/" + item.Name},
			Properties: pluginapi.PropertyMap{
				propertyKeyDescription: item.Description,
			},
		}
		nodes = append(nodes, modelNode)

		for _, version := range item.LatestVersions {
			if strings.TrimSpace(version.RunID) == "" {
				continue
			}

			run, err := p.fetchRun(ctx, version.RunID)
			if err != nil {
				fetchRunErrors = append(fetchRunErrors, fmt.Errorf("fetch mlflow run %s for model %s version %s: %w", version.RunID, item.Name, version.Version, err))
				continue
			}

			for _, datasetInput := range run.Inputs.DatasetInputs {
				datasetName := strings.TrimSpace(datasetInput.Dataset.Name)
				if datasetName == "" {
					continue
				}

				datasetPath := p.config.PathPrefix + "/" + datasetName
				datasetKey := pluginapi.NodeKindDataset + ":" + datasetPath
				if _, ok := seenDatasets[datasetKey]; !ok {
					seenDatasets[datasetKey] = pluginapi.NodeClaim{
						ID: pluginapi.NodeID{Kind: pluginapi.NodeKindDataset, Path: datasetPath},
						Properties: pluginapi.PropertyMap{
							propertyKeyDigest:     datasetInput.Dataset.Digest,
							propertyKeySourceType: datasetInput.Dataset.SourceType,
						},
					}
				}

				relations = append(relations, pluginapi.RelationClaim{
					Kind: pluginapi.RelationKindTrainedOn,
					From: modelNode.ID,
					To:   pluginapi.NodeID{Kind: pluginapi.NodeKindDataset, Path: datasetPath},
					Properties: pluginapi.PropertyMap{
						propertyKeyRunID: version.RunID,
					},
				})
			}
		}
	}

	for _, dataset := range seenDatasets {
		nodes = append(nodes, dataset)
	}

	response := pluginapi.CollectResponse{Nodes: nodes, Relations: relations}
	if len(fetchRunErrors) > 0 {
		return response, errors.Join(fetchRunErrors...)
	}

	return response, nil
}

func (p *Plugin) fetchRegisteredModels(ctx context.Context) ([]registeredModel, error) {
	url, err := httpjson.BuildURL(p.config.BaseURL+"/api/2.0/mlflow/registered-models/search", url.Values{
		"max_results": {"1000"},
		"order_by":    {"name ASC"},
	})
	if err != nil {
		return nil, fmt.Errorf("building MLflow registered models URL: %w", err)
	}

	var payload registeredModelsResponse
	err = httpjson.Get(ctx, p.httpClient, url.String(), &payload,
		httpjson.WithAuthorizationBearer(p.config.BearerToken))
	if err != nil {
		return nil, fmt.Errorf("calling MLflow registered models endpoint: %w", err)
	}

	return payload.RegisteredModels, nil
}

func (p *Plugin) fetchRun(ctx context.Context, runID string) (run, error) {
	url, err := httpjson.BuildURL(p.config.BaseURL+"/api/2.0/mlflow/runs/get", url.Values{
		"run_id": {runID},
	})
	if err != nil {
		return run{}, fmt.Errorf("building MLflow run URL for %q: %w", runID, err)
	}

	var payload getRunResponse
	err = httpjson.Get(ctx, p.httpClient, url.String(), &payload,
		httpjson.WithAuthorizationBearer(p.config.BearerToken))
	if err != nil {
		return run{}, fmt.Errorf("calling MLflow run endpoint for %q: %w", runID, err)
	}

	return payload.Run, nil
}

type registeredModelsResponse struct {
	RegisteredModels []registeredModel `json:"registered_models"`
}

type registeredModel struct {
	Name                 string         `json:"name"`
	Description          string         `json:"description"`
	CreationTimestamp    int64          `json:"creation_timestamp"`
	LastUpdatedTimestamp int64          `json:"last_updated_timestamp"`
	LatestVersions       []modelVersion `json:"latest_versions"`
	Tags                 []modelTag     `json:"tags"`
}

type modelVersion struct {
	Version      string `json:"version"`
	CurrentStage string `json:"current_stage"`
	RunID        string `json:"run_id"`
	Source       string `json:"source"`
}

type modelTag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type getRunResponse struct {
	Run run `json:"run"`
}

type run struct {
	Inputs runInputs `json:"inputs"`
}

type runInputs struct {
	DatasetInputs []datasetInput `json:"dataset_inputs"`
}

type datasetInput struct {
	Dataset dataset `json:"dataset"`
}

type dataset struct {
	Name       string `json:"name"`
	Digest     string `json:"digest"`
	SourceType string `json:"source_type"`
}
