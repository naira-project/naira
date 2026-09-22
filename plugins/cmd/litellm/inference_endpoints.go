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
	"time"

	"github.com/naira-project/naira/plugins/pkg/pluginapi"
)

const (
	propertyKeyModelID                    = "model_id"
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
	propertyKeyInvocations                = "invocations_total"

	endpointTypeInternal = "internal"
	endpointTypeExternal = "external"

	endpointStatusHealthy   = "healthy"
	endpointStatusUnhealthy = "unhealthy"
	endpointStatusUnknown   = "unknown"
)

type inferenceEndpoint struct {
	ModelID            string  `json:"model_id"`
	EndpointType       string  `json:"endpoint_type"`
	Provider           string  `json:"provider"`
	Status             string  `json:"status"`
	APIProtocol        string  `json:"api_protocol"`
	EndpointURL        string  `json:"endpoint_url"`
	Region             string  `json:"region"`
	ServesModel        string  `json:"model_name"`
	OwnedBy            string  `json:"owned_by"`
	LifecycleStatus    string  `json:"lifecycle_status"`
	DiscoveredVia      string  `json:"discovered_via"`
	LastSeen           string  `json:"last_seen"`
	Mode               string  `json:"mode"`
	MaxTokens          int64   `json:"max_tokens"`
	InputCostPerToken  float64 `json:"input_cost_per_token"`
	OutputCostPerToken float64 `json:"output_cost_per_token"`
	Invocations        int64   `json:"-"`
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
	ModelID            string  `json:"id"`
	Mode               string  `json:"mode"`
	MaxTokens          int64   `json:"max_tokens"`
	InputCostPerToken  float64 `json:"input_cost_per_token"`
	OutputCostPerToken float64 `json:"output_cost_per_token"`
}

type healthResponse struct {
	HealthyEndpoints   []healthEndpointEntry `json:"healthy_endpoints"`
	UnhealthyEndpoints []healthEndpointEntry `json:"unhealthy_endpoints"`
}

type healthEndpointEntry struct {
	Model   string `json:"model"`
	APIBase string `json:"api_base"`
}

type modelAndAPIBase struct{ model, apiBase string }

func (p *Plugin) listInferenceEndpoints(ctx context.Context, ownerByModelID map[string]string) ([]pluginapi.NodeClaim, []pluginapi.RelationClaim, error) {

	statusByKey, err := p.fetchEndpointHealth(ctx)
	if err != nil {
		if p.logger != nil {
			p.logger.Printf("WARN: fetching LiteLLM endpoint health failed, marking endpoints as status unknown: %v", err)
		}
	}

	endpoints, err := p.fetchInferenceEndpoints(ctx, statusByKey)
	if err != nil {
		return nil, nil, fmt.Errorf("listing inference endpoints: %w", err)
	}

	invocations, err := p.fetchModelInvocations(ctx)
	if err != nil {
		return []pluginapi.NodeClaim{}, []pluginapi.RelationClaim{}, fmt.Errorf("Error while fetching model invocations: %v", err)
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

		// Only models with recorded API requests in the lookback window are
		// treated as actively serving endpoints.
		if invocations[modelName] == 0 {
			continue
		}
		endpoint.Invocations = invocations[modelName]

		endpoint.OwnedBy = ownerByModelID[modelName]

		regionSuffix := ""
		if region := strings.TrimSpace(endpoint.Region); region != "" {
			regionSuffix = "-" + region
		}

		endpointNode := pluginapi.NodeClaim{
			ID: pluginapi.NodeID{
				Kind: pluginapi.NodeKindInferenceEndpoint,
				Path: p.config.PathPrefix + "/" + modelName + regionSuffix,
			},
			Properties: endpoint.properties(),
		}
		nodes = append(nodes, endpointNode)

		relations = append(relations, pluginapi.RelationClaim{
			Kind: pluginapi.RelationKindServesModel,
			From: endpointNode.ID,
			To: pluginapi.NodeID{
				Kind: pluginapi.NodeKindModel,
				Path: p.config.PathPrefix + "/" + modelName,
			},
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
		propertyKeyModelID:         e.ModelID,
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
	if e.InputCostPerToken != 0 {
		properties[propertyKeyInputCostPerMillionTokens] = strconv.FormatFloat(e.InputCostPerToken*1_000_000, 'f', 4, 64)
	}
	if e.OutputCostPerToken != 0 {
		properties[propertyKeyOutputCostPerMillionTokens] = strconv.FormatFloat(e.OutputCostPerToken*1_000_000, 'f', 4, 64)
	}
	if e.Invocations != 0 {
		properties[propertyKeyInvocations] = strconv.FormatInt(e.Invocations, 10)
	}

	return properties
}

// getLiteLLMJSON issues an authorized GET against urlStr and decodes the JSON
// response body into out. name and path identify the endpoint in error
// messages, e.g. name "health" and path "/health".
func (p *Plugin) getLiteLLMJSON(ctx context.Context, urlStr, name, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return fmt.Errorf("building LiteLLM %s request: %w", name, err)
	}
	p.addAuthorization(req)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("calling LiteLLM %s endpoint: %w", name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("litellm %s returned %s", path, resp.Status)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding LiteLLM %s response: %w", name, err)
	}

	return nil
}

func (p *Plugin) fetchInferenceEndpoints(ctx context.Context, statusByKey map[modelAndAPIBase]string) ([]inferenceEndpoint, error) {
	var payload struct {
		Data []struct {
			ModelName     string           `json:"model_name"`
			LiteLLMParams modelInfoLiteLLM `json:"litellm_params"`
			ModelInfo     modelInfoDetail  `json:"model_info"`
		} `json:"data"`
	}
	if err := p.getLiteLLMJSON(ctx, p.config.BaseURL+"/model/info", "Model info", "/model/info", &payload); err != nil {
		return nil, err
	}

	endpoints := make([]inferenceEndpoint, 0, len(payload.Data))
	for _, entry := range payload.Data {
		status, ok := statusByKey[modelAndAPIBase{
			model:   strings.TrimSpace(entry.LiteLLMParams.Model),
			apiBase: strings.TrimSpace(entry.LiteLLMParams.APIBase),
		}]
		if !ok {
			status = endpointStatusUnknown
		}

		endpoints = append(endpoints, inferenceEndpoint{
			ModelID:            entry.ModelInfo.ModelID,
			Provider:           entry.LiteLLMParams.provider(),
			EndpointType:       entry.LiteLLMParams.endpointType(),
			Status:             status,
			EndpointURL:        entry.LiteLLMParams.APIBase,
			Region:             entry.LiteLLMParams.RegionName,
			ServesModel:        strings.TrimSpace(entry.ModelName),
			Mode:               entry.ModelInfo.Mode,
			MaxTokens:          entry.ModelInfo.MaxTokens,
			InputCostPerToken:  entry.ModelInfo.InputCostPerToken,
			OutputCostPerToken: entry.ModelInfo.OutputCostPerToken,
		})
	}

	return endpoints, nil
}

// fetchEndpointHealth calls LiteLLM's /health endpoint and returns a map of
// endpoint (model + api_base) to status, so it can be joined onto the
// endpoints returned by /model/info.
func (p *Plugin) fetchEndpointHealth(ctx context.Context) (map[modelAndAPIBase]string, error) {
	var payload healthResponse
	if err := p.getLiteLLMJSON(ctx, p.config.BaseURL+"/health", "health", "/health", &payload); err != nil {
		return nil, err
	}

	status := make(map[modelAndAPIBase]string, len(payload.HealthyEndpoints)+len(payload.UnhealthyEndpoints))
	for _, entry := range payload.UnhealthyEndpoints {
		status[modelAndAPIBase{
			model:   strings.TrimSpace(entry.Model),
			apiBase: strings.TrimSpace(entry.APIBase),
		}] = endpointStatusUnhealthy
	}
	for _, entry := range payload.HealthyEndpoints {
		status[modelAndAPIBase{
			model:   strings.TrimSpace(entry.Model),
			apiBase: strings.TrimSpace(entry.APIBase),
		}] = endpointStatusHealthy
	}

	return status, nil
}

// fetchModelInvocations queries LiteLLM's /user/daily/activity for the
// configured lookback window, without a user_id filter, so the master key
// gets api_requests summed across all users for each model.
// TODO: configure fetch mechanism to be scoped to a specific id, however it is to be talked
// together with the authorization mechanism within Naira (i.e. how to map LiteLLM credentials with Naira user).
func (p *Plugin) fetchModelInvocations(ctx context.Context) (map[string]int64, error) {
	lookback := p.config.MetricsLookback
	endDate := time.Now().UTC()
	startDate := endDate.Add(-lookback)

	requestURL, err := url.Parse(p.config.BaseURL + "/user/daily/activity")
	if err != nil {
		return nil, fmt.Errorf("building LiteLLM daily activity request: %w", err)
	}
	query := requestURL.Query()
	query.Set("start_date", startDate.Format("2006-01-02"))
	query.Set("end_date", endDate.Format("2006-01-02"))
	requestURL.RawQuery = query.Encode()

	var payload struct {
		Results []struct {
			Breakdown struct {
				ModelGroups map[string]struct {
					Metrics struct {
						APIRequests int64 `json:"api_requests"`
					} `json:"metrics"`
				} `json:"model_groups"`
			} `json:"breakdown"`
		} `json:"results"`
	}
	if err := p.getLiteLLMJSON(ctx, requestURL.String(), "daily activity", "/user/daily/activity", &payload); err != nil {
		return nil, err
	}

	invocations := make(map[string]int64)
	for _, record := range payload.Results {
		for modelName, entry := range record.Breakdown.ModelGroups {
			invocations[modelName] += entry.Metrics.APIRequests
		}
	}

	return invocations, nil
}
