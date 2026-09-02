package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/naira-project/naira/plugins/pkg/pluginapi"
)

const (
	propertyKeyEndpointType               = "endpoint_type"
	propertyKeyProvider                   = "provider"
	propertyKeyEndpointStatus             = "status"
	propertyKeyAPIProtocol                = "api_protocol"
	propertyKeyEndpointURL                = "endpoint_url"
	propertyKeyRegion                     = "region"
	propertyKeyServesModel                = "serves_model"
	propertyKeyLifecycleStatus            = "lifecycle_status"
	propertyKeyLastSeen                   = "last_seen"
	propertyKeyMode                       = "mode"
	propertyKeyMaxTokens                  = "max_tokens"
	propertyKeyInputCostPerMillionTokens  = "input_cost_per_million_tokens"
	propertyKeyOutputCostPerMillionTokens = "output_cost_per_million_tokens"

	endpointTypeInternal = "internal"
	endpointTypeExternal = "external"
)

type inferenceEndpoint struct {
	EndpointType    string  `json:"endpoint_type"`
	Provider        string  `json:"provider"`
	Status          string  `json:"status"`
	APIProtocol     string  `json:"api_protocol"`
	EndpointURL     string  `json:"endpoint_url"`
	Region          string  `json:"region"`
	ServesModel     string  `json:"model_name"`
	OwnedBy         string  `json:"owned_by"`
	LifecycleStatus string  `json:"lifecycle_status"`
	DiscoveredVia   string  `json:"discovered_via"`
	LastSeen        string  `json:"last_seen"`
	Mode            string  `json:"mode"`
	MaxTokens       int64   `json:"max_tokens"`
	InputCost       float64 `json:"input_cost_per_token"`
	OutputCost      float64 `json:"output_cost_per_token"`
}

type modelInfoResponse struct {
	Data []modelInfoEntry `json:"data"`
}

type modelInfoEntry struct {
	ModelName     string           `json:"model_name"`
	LiteLLMParams modelInfoLiteLLM `json:"litellm_params"`
	ModelInfo     modelInfoDetail  `json:"model_info"`
}

type modelInfoLiteLLM struct {
	Model             string `json:"model"`
	APIBase           string `json:"api_base"`
	CustomLLMProvider string `json:"custom_llm_provider"`
	RegionName        string `json:"region_name"`
}

type modelInfoDetail struct {
	ID                 string  `json:"id"`
	Mode               string  `json:"mode"`
	MaxTokens          int64   `json:"max_tokens"`
	InputCostPerToken  float64 `json:"input_cost_per_token"`
	OutputCostPerToken float64 `json:"output_cost_per_token"`
}

func (p *Plugin) listInferenceEndpoints(ctx context.Context, ownedByModel map[string]string) ([]pluginapi.NodeClaim, []pluginapi.RelationClaim, error) {
	endpoints, err := p.fetchInferenceEndpoints(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("listing inference endpoints: %w", err)
	}

	var (
		nodes     []pluginapi.NodeClaim
		relations []pluginapi.RelationClaim
	)

	for _, endpoint := range endpoints {
		modelName := strings.TrimSpace(endpoint.ServesModel)
		if modelName == "" {
			if p.logger != nil {
				p.logger.Printf("WARN: skipping inference endpoint with no model name")
			}
			continue
		}

		endpoint.OwnedBy = ownedByModel[modelName]

		endpointKey := modelName
		if region := strings.TrimSpace(endpoint.Region); region != "" {
			endpointKey += "-" + region
		}

		endpointNode := pluginapi.NodeClaim{
			ID:         pluginapi.NodeID{Kind: pluginapi.NodeKindInferenceEndpoint, Path: p.config.PathPrefix + "/" + endpointKey},
			Properties: endpoint.properties(),
		}
		nodes = append(nodes, endpointNode)

		relations = append(relations, pluginapi.RelationClaim{
			Kind: pluginapi.RelationKindServesModel,
			From: endpointNode.ID,
			To:   pluginapi.NodeID{Kind: pluginapi.NodeKindModel, Path: p.config.PathPrefix + "/" + modelName},
		})
	}

	return nodes, relations, nil
}

// endpointType distinguishes a self-hosted deployment reachable only inside the
// cluster from an externally managed SaaS endpoint, based on the api_base host.
func (m modelInfoLiteLLM) endpointType() string {
	base := strings.TrimSpace(m.APIBase)
	if base == "" {
		return endpointTypeExternal
	}
	parsedURL, err := url.Parse(base)
	if err != nil {
		return endpointTypeExternal
	}
	host := parsedURL.Hostname()
	if host == "localhost" || strings.HasSuffix(host, ".svc") || strings.HasSuffix(host, ".svc.cluster.local") {
		return endpointTypeInternal
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsPrivate() || ip.IsLoopback()) {
		return endpointTypeInternal
	}
	return endpointTypeExternal
}

func (m modelInfoLiteLLM) provider() string {
	if p := strings.TrimSpace(m.CustomLLMProvider); p != "" {
		return p
	}
	if provider, _, ok := strings.Cut(m.Model, "/"); ok {
		return strings.TrimSpace(provider)
	}
	return ""
}

func (e inferenceEndpoint) properties() pluginapi.PropertyMap {
	properties := pluginapi.PropertyMap{}
	for key, value := range map[string]string{
		propertyKeyEndpointType:    e.EndpointType,
		propertyKeyProvider:        e.Provider,
		propertyKeyEndpointStatus:  e.Status,
		propertyKeyAPIProtocol:     e.APIProtocol,
		propertyKeyEndpointURL:     e.EndpointURL,
		propertyKeyRegion:          e.Region,
		propertyKeyServesModel:     e.ServesModel,
		propertyKeyOwnedBy:         e.OwnedBy,
		propertyKeyLifecycleStatus: e.LifecycleStatus,
		propertyKeyDiscoveredVia:   e.DiscoveredVia,
		propertyKeyLastSeen:        e.LastSeen,
		propertyKeyMode:            e.Mode,
	} {
		if value != "" {
			properties[key] = value
		}
	}

	if e.MaxTokens != 0 {
		properties[propertyKeyMaxTokens] = strconv.FormatInt(e.MaxTokens, 10)
	}
	if e.InputCost != 0 {
		properties[propertyKeyInputCostPerMillionTokens] = strconv.FormatFloat(e.InputCost*1_000_000, 'f', 4, 64)
	}
	if e.OutputCost != 0 {
		properties[propertyKeyOutputCostPerMillionTokens] = strconv.FormatFloat(e.OutputCost*1_000_000, 'f', 4, 64)
	}

	return properties
}

func (p *Plugin) fetchInferenceEndpoints(ctx context.Context) ([]inferenceEndpoint, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.config.BaseURL+"/model/info", nil)
	if err != nil {
		return nil, fmt.Errorf("building LiteLLM Model info request: %w", err)
	}
	p.addAuthorization(req)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling LiteLLM Model info endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("litellm /model/info returned %s", resp.Status)
	}

	var payload modelInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding LiteLLM Model info response: %w", err)
	}

	endpoints := make([]inferenceEndpoint, 0, len(payload.Data))
	for _, entry := range payload.Data {
		endpoints = append(endpoints, inferenceEndpoint{
			Provider:     entry.LiteLLMParams.provider(),
			EndpointType: entry.LiteLLMParams.endpointType(),
			EndpointURL:  entry.LiteLLMParams.APIBase,
			Region:       entry.LiteLLMParams.RegionName,
			ServesModel:  strings.TrimSpace(entry.ModelName),
			Mode:         entry.ModelInfo.Mode,
			MaxTokens:    entry.ModelInfo.MaxTokens,
			InputCost:    entry.ModelInfo.InputCostPerToken,
			OutputCost:   entry.ModelInfo.OutputCostPerToken,
		})
	}

	return endpoints, nil
}