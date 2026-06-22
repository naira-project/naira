package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/naira-project/naira/catalog/pluginapi"
)

const (
	pluginName = "mlflow"

	propertyKeyDigest      = "digest"
	propertyKeyRunID       = "run_id"
	propertyKeySourceType  = "source_type"
	propertyKeyDescription = "description"
)

type Plugin struct {
	httpClient *http.Client
	config     pluginConfig
}

func New(httpClient *http.Client, config pluginConfig) *Plugin {
	return &Plugin{
		httpClient: httpClient,
		config:     config,
	}
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
			ID: pluginapi.NodeID{Kind: pluginapi.NodeKindModel, Path: pluginName + "/" + item.Name},
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

				datasetPath := pluginName + "/" + datasetName
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
	endpoint, err := url.Parse(p.config.BaseURL + "/api/2.0/mlflow/registered-models/search")
	if err != nil {
		return nil, fmt.Errorf("building MLflow registered models URL: %w", err)
	}

	query := endpoint.Query()
	query.Set("max_results", "1000")
	query.Set("order_by", "name ASC")
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("building MLflow registered models request: %w", err)
	}
	p.addAuthorization(req)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling MLflow registered models endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("mlflow registered models returned %s", resp.Status)
	}

	var payload registeredModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding MLflow registered models response: %w", err)
	}

	return payload.RegisteredModels, nil
}

func (p *Plugin) fetchRun(ctx context.Context, runID string) (run, error) {
	endpoint, err := url.Parse(p.config.BaseURL + "/api/2.0/mlflow/runs/get")
	if err != nil {
		return run{}, fmt.Errorf("building MLflow run URL for %q: %w", runID, err)
	}

	query := endpoint.Query()
	query.Set("run_id", runID)
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return run{}, fmt.Errorf("building MLflow run request for %q: %w", runID, err)
	}
	p.addAuthorization(req)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return run{}, fmt.Errorf("calling MLflow run endpoint for %q: %w", runID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return run{}, fmt.Errorf("mlflow run %s returned %s", runID, resp.Status)
	}

	var payload getRunResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return run{}, fmt.Errorf("decoding MLflow run response for %q: %w", runID, err)
	}

	return payload.Run, nil
}

func (p *Plugin) addAuthorization(req *http.Request) {
	if strings.TrimSpace(p.config.BearerToken) != "" {
		req.Header.Set("Authorization", "Bearer "+p.config.BearerToken)
	}
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
