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
	propertyKeyID                    		= 		"id"
	propertyKeyEndpointType               = "endpoint_type"
	propertyKeyProvider                   = "provider"
	/* In mcp.go, propertyKeyStatus is already used, preventing this file from 
	* using propertyKeyStatus. LiteLLM API returns  Hence, here, for displaying the status of a single inference endpoint in Naira
	* (healthy/unhealthy/unknown), propertyKeyEndpointStatus with the value 'status' will be used.		
	*/
	propertyKeyEndpointStatus             = "status"
	propertyKeyAPIProtocol                = "api_protocol"
	propertyKeyAPIBase               = "api_base"
	propertyKeyRegionName                   = "region_name"
	propertyKeyModelName                  = "model_name"
	propertyKeyLifecycleStatus            = "lifecycle_status"
	propertyKeyLastSeen                   = "last_seen"
	propertyKeyMode                       = "mode"
	propertyKeyMaxTokens                  = "max_tokens"
	propertyKeyInputCostPerMillionTokens  = "input_cost_per_million_tokens"
	propertyKeyOutputCostPerMillionTokens = "output_cost_per_million_tokens"
	propertyKeyInvocationsTotal           = "invocations_total"

	endpointTypeInternal = "internal"
	endpointTypeExternal = "external"

	endpointStatusHealthy   = "healthy"
	endpointStatusUnhealthy = "unhealthy"
	endpointStatusUnknown   = "unknown"
)

type inferenceEndpoint struct {
	ID            	   string
	EndpointType       string
	Provider           string
	Status             string
	APIProtocol        string
	APIBase        	   string
	RegionName         string
	ModelName          string
	OwnedBy            string
	LifecycleStatus    string
	DiscoveredVia      string
	LastSeen           string
	Mode               string
	MaxTokens          int64
	InputCostPerToken  float64
	OutputCostPerToken float64
	Invocations int64
}

type litellmParams struct {
	modelAndAPIBase
	CustomLLMProvider string `json:"custom_llm_provider"`
	RegionName        string `json:"region_name"`
}

type modelInfo struct {
	ID                 string  `json:"id"`
	Mode               string  `json:"mode"`
	MaxTokens          int64   `json:"max_tokens"`
	InputCostPerToken  float64 `json:"input_cost_per_token"`
	OutputCostPerToken float64 `json:"output_cost_per_token"`
}

type healthResponse struct {
	HealthyEndpoints   []modelAndAPIBase `json:"healthy_endpoints"`
	UnhealthyEndpoints []modelAndAPIBase `json:"unhealthy_endpoints"`
}

type modelAndAPIBase struct {
	Model   string `json:"model"`
	APIBase string `json:"api_base"`
}

func (m modelAndAPIBase) normalizedMAAB() modelAndAPIBase {
	return modelAndAPIBase{
		Model:   strings.TrimSpace(m.Model),
		APIBase: strings.TrimSpace(m.APIBase),
	}
}

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
		return nil, nil, fmt.Errorf("fetching model invocations: %w", err)
	}

	var (
		nodes     []pluginapi.NodeClaim
		relations []pluginapi.RelationClaim
	)

	for _, endpoint := range endpoints {
		modelName := strings.TrimSpace(endpoint.ModelName)
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
		if region := strings.TrimSpace(endpoint.RegionName); region != "" {
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
func (m litellmParams) endpointType() string {
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

func (m litellmParams) provider() string {
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
		propertyKeyID:         e.ID,
		propertyKeyEndpointType:    e.EndpointType,
		propertyKeyProvider:        e.Provider,
		propertyKeyEndpointStatus:  e.Status,
		propertyKeyAPIProtocol:     e.APIProtocol,
		propertyKeyAPIBase:     e.APIBase,
		propertyKeyRegionName:          e.RegionName,
		propertyKeyModelName:       e.ModelName,
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
		properties[propertyKeyInvocationsTotal] = strconv.FormatInt(e.Invocations, 10)
	}

	return properties
}

// getLiteLLMJSON issues an authorized GET against urlStr and decodes the JSON
// response body into out. Callers wrap the returned error with the endpoint
// they called.
func (p *Plugin) getLiteLLMJSON(ctx context.Context, urlStr string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	p.addAuthorization(req)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %s", resp.Status)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding response body: %w", err)
	}

	return nil
}

func (p *Plugin) fetchInferenceEndpoints(ctx context.Context, statusByKey map[modelAndAPIBase]string) ([]inferenceEndpoint, error) {
	var payload struct {
		Data []struct {
			ModelName     string        `json:"model_name"`
			LiteLLMParams litellmParams `json:"litellm_params"`
			ModelInfo     modelInfo     `json:"model_info"`
		} `json:"data"`
	}
	if err := p.getLiteLLMJSON(ctx, p.config.BaseURL+"/model/info", &payload); err != nil {
		return nil, fmt.Errorf("fetching LiteLLM /model/info: %w", err)
	}

	endpoints := make([]inferenceEndpoint, 0, len(payload.Data))
	for _, entry := range payload.Data {
		status, ok := statusByKey[entry.LiteLLMParams.normalizedMAAB()]
		if !ok {
			status = endpointStatusUnknown
		}

		endpoints = append(endpoints, inferenceEndpoint{
			ID:            entry.ModelInfo.ID,
			Provider:           entry.LiteLLMParams.provider(),
			EndpointType:       entry.LiteLLMParams.endpointType(),
			Status:             status,
			APIBase:        entry.LiteLLMParams.APIBase,
			RegionName:             entry.LiteLLMParams.RegionName,
			ModelName:          strings.TrimSpace(entry.ModelName),
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
	if err := p.getLiteLLMJSON(ctx, p.config.BaseURL+"/health", &payload); err != nil {
		return nil, fmt.Errorf("fetching LiteLLM /health: %w", err)
	}

	status := make(map[modelAndAPIBase]string, len(payload.HealthyEndpoints)+len(payload.UnhealthyEndpoints))
	for _, entry := range payload.UnhealthyEndpoints {
		status[entry.normalizedMAAB()] = endpointStatusUnhealthy
	}
	for _, entry := range payload.HealthyEndpoints {
		status[entry.normalizedMAAB()] = endpointStatusHealthy
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
	if err := p.getLiteLLMJSON(ctx, requestURL.String(), &payload); err != nil {
		return nil, fmt.Errorf("fetching LiteLLM /user/daily/activity: %w", err)
	}

	invocations := make(map[string]int64)
	for _, record := range payload.Results {
		for modelName, entry := range record.Breakdown.ModelGroups {
			invocations[modelName] += entry.Metrics.APIRequests
		}
	}

	return invocations, nil
}
