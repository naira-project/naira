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
	propertyKeyID           = "id"
	propertyKeyEndpointType = "endpoint_type"
	propertyKeyProvider     = "provider"
	/* In mcp.go, propertyKeyStatus is already used, preventing this file from
	* using propertyKeyStatus. LiteLLM API returns  Hence, here, for displaying the status of a single inference endpoint in Naira
	* (healthy/unhealthy/unknown), propertyKeyEndpointStatus with the value 'status' will be used.
	 */
	propertyKeyEndpointStatus             = "status"
	propertyKeyAPIBase                    = "api_base"
	propertyKeyRegionName                 = "region_name"
	propertyKeyModelName                  = "model_name"
	propertyKeyLifecycleStatus            = "lifecycle_status"
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

	lifecycleStatusActive = "active"
)

type inferenceEndpoint struct {
	ModelName string
	// Props holds the properties already converted to their final string
	// form (provider, status, costs, ...), ready to be merged into a
	// node's PropertyMap.
	Props pluginapi.PropertyMap
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
		modelName := endpoint.ModelName
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
		endpoint.Props[propertyKeyInvocationsTotal] = strconv.FormatInt(invocations[modelName], 10)
		endpoint.Props[propertyKeyLifecycleStatus] = lifecycleStatusActive

		if owner := ownerByModelID[modelName]; owner != "" {
			endpoint.Props[propertyKeyOwnedBy] = owner
		}

		// Several deployments can serve the same model_name, so the path is keyed
		// by the deployment's model_info.id. The region, when set, is included
		// only to make the endpoint's name readable in the UI. Both are folded
		// into the last segment rather than added as their own, since the UI
		// reads the second-to-last segment as the endpoint's source.
		endpointName := modelName
		if regionName := endpoint.Props[propertyKeyRegionName]; regionName != "" {
			endpointName += "-" + regionName
		}
		if id := endpoint.Props[propertyKeyID]; id != "" {
			endpointName += "-" + id
		}

		endpointNode := pluginapi.NodeClaim{
			ID: pluginapi.NodeID{
				Kind: pluginapi.NodeKindInferenceEndpoint,
				Path: p.config.PathPrefix + "/" + endpointName,
			},
			Properties: endpoint.Props,
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
	if customLLMProvider := strings.TrimSpace(m.CustomLLMProvider); customLLMProvider != "" {
		return customLLMProvider
	}
	if provider, _, ok := strings.Cut(m.Model, "/"); ok {
		return strings.TrimSpace(provider)
	}
	return ""
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
		endpointStatus, ok := statusByKey[entry.LiteLLMParams.normalizedMAAB()]
		if !ok {
			endpointStatus = endpointStatusUnknown
		}

		modelName := strings.TrimSpace(entry.ModelName)

		props := pluginapi.PropertyMap{}
		for key, value := range map[string]string{
			propertyKeyID:             strings.TrimSpace(entry.ModelInfo.ID),
			propertyKeyEndpointType:   entry.LiteLLMParams.endpointType(),
			propertyKeyProvider:       entry.LiteLLMParams.provider(),
			propertyKeyEndpointStatus: endpointStatus,
			propertyKeyAPIBase:        entry.LiteLLMParams.APIBase,
			propertyKeyRegionName:     strings.TrimSpace(entry.LiteLLMParams.RegionName),
			propertyKeyModelName:      modelName,
			propertyKeyMode:           entry.ModelInfo.Mode,
		} {
			if value != "" {
				props[key] = value
			}
		}

		if entry.ModelInfo.MaxTokens != 0 {
			props[propertyKeyMaxTokens] = strconv.FormatInt(entry.ModelInfo.MaxTokens, 10)
		}
		if entry.ModelInfo.InputCostPerToken != 0 {
			props[propertyKeyInputCostPerMillionTokens] = strconv.FormatFloat(entry.ModelInfo.InputCostPerToken*1_000_000, 'f', 4, 64)
		}
		if entry.ModelInfo.OutputCostPerToken != 0 {
			props[propertyKeyOutputCostPerMillionTokens] = strconv.FormatFloat(entry.ModelInfo.OutputCostPerToken*1_000_000, 'f', 4, 64)
		}

		endpoints = append(endpoints, inferenceEndpoint{
			ModelName: modelName,
			Props:     props,
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

	endpointStatus := make(map[modelAndAPIBase]string, len(payload.HealthyEndpoints)+len(payload.UnhealthyEndpoints))
	for _, entry := range payload.UnhealthyEndpoints {
		endpointStatus[entry.normalizedMAAB()] = endpointStatusUnhealthy
	}
	for _, entry := range payload.HealthyEndpoints {
		endpointStatus[entry.normalizedMAAB()] = endpointStatusHealthy
	}

	return endpointStatus, nil
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
