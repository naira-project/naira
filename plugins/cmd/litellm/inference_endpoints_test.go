package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startLiteLLMModelInfo serves /model/info, /health and /user/daily/activity
// with the given payloads, mirroring the endpoints listInferenceEndpoints
// depends on. invocationsByModel maps model name to api_requests; a model
// missing from it is treated as having received no traffic.
func startLiteLLMModelInfo(t *testing.T, modelInfo modelInfoResponse, health *healthResponse, invocationsByModel map[string]int64) string {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/model/info", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(modelInfo))
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		if health == nil {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(health))
	})
	mux.HandleFunc("/user/daily/activity", func(w http.ResponseWriter, _ *http.Request) {
		var modelGroups []string
		for modelName, count := range invocationsByModel {
			modelGroups = append(modelGroups, fmt.Sprintf(`"%s": {"metrics": {"api_requests": %d}}`, modelName, count))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{
  "results": [{
    "breakdown": {
      "model_groups": {%s}
    }
  }]
}`, strings.Join(modelGroups, ","))
	})

	httpServer := httptest.NewServer(mux)
	t.Cleanup(httpServer.Close)

	return httpServer.URL
}

func TestListInferenceEndpointsEmitsNodesAndRelations(t *testing.T) {
	baseURL := startLiteLLMModelInfo(t,
		modelInfoResponse{Data: []modelInfoEntry{
			{
				ModelName: "idp-claude-sonnet",
				LiteLLMParams: modelInfoLiteLLM{
					Model:      "anthropic/claude-3-5-sonnet-latest",
					APIBase:    "https://api.anthropic.com",
					RegionName: "us-east-1",
				},
				ModelInfo: modelInfoDetail{
					ModelID: "model-1", Mode: "chat", MaxTokens: 8192,
					InputCostPerToken: 0.000003, OutputCostPerToken: 0.000015,
				},
			},
		}},
		&healthResponse{HealthyEndpoints: []healthEndpointEntry{
			{Model: "anthropic/claude-3-5-sonnet-latest", APIBase: "https://api.anthropic.com"},
		}},
		map[string]int64{"idp-claude-sonnet": 7},
	)

	nodes, relations, err := testPlugin(t, baseURL).
		listInferenceEndpoints(t.Context(), map[string]string{"idp-claude-sonnet": "team-a"})
	require.NoError(t, err)

	endpoints := nodePaths(nodes, pluginapi.NodeKindInferenceEndpoint)
	require.Contains(t, endpoints, "litellm/idp-claude-sonnet-us-east-1",
		"the region is appended to disambiguate endpoints serving the same model")

	endpoint := endpoints["litellm/idp-claude-sonnet-us-east-1"]
	assert.Equal(t, "model-1", endpoint[propertyKeyModelID])
	assert.Equal(t, "7", endpoint[propertyKeyInvocations])
	assert.Equal(t, "anthropic", endpoint[propertyKeyProvider])
	assert.Equal(t, endpointTypeExternal, endpoint[propertyKeyEndpointType])
	assert.Equal(t, endpointStatusHealthy, endpoint[propertyKeyEndpointStatus])
	assert.Equal(t, "https://api.anthropic.com", endpoint[propertyKeyEndpointURL])
	assert.Equal(t, "us-east-1", endpoint[propertyKeyRegion])
	assert.Equal(t, "idp-claude-sonnet", endpoint[propertyKeyServesModel])
	assert.Equal(t, "team-a", endpoint[propertyKeyOwnedBy])
	assert.Equal(t, "chat", endpoint[propertyKeyMode])
	assert.Equal(t, "8192", endpoint[propertyKeyMaxTokens])
	assert.Equal(t, "3.0000", endpoint[propertyKeyInputCostPerMillionTokens])
	assert.Equal(t, "15.0000", endpoint[propertyKeyOutputCostPerMillionTokens])

	require.Len(t, relations, 1)
	assert.Equal(t, pluginapi.RelationKindServesModel, relations[0].Kind)
	assert.Equal(t, pluginapi.NodeID{Kind: pluginapi.NodeKindModel, Path: "litellm/idp-claude-sonnet"}, relations[0].To)
}

func TestListInferenceEndpointsSkipsModelsWithNoInvocations(t *testing.T) {
	baseURL := startLiteLLMModelInfo(t,
		modelInfoResponse{Data: []modelInfoEntry{{ModelName: "idp-unused-model"}}},
		&healthResponse{HealthyEndpoints: []healthEndpointEntry{{Model: "idp-unused-model"}}},
		nil,
	)

	nodes, relations, err := testPlugin(t, baseURL).listInferenceEndpoints(t.Context(), nil)
	require.NoError(t, err)
	assert.Empty(t, nodes, "/model/info lists every configured deployment, not just ones actually receiving traffic")
	assert.Empty(t, relations)
}

func TestListInferenceEndpointsSkipsEntryWithNoModelName(t *testing.T) {
	baseURL := startLiteLLMModelInfo(t,
		modelInfoResponse{Data: []modelInfoEntry{{ModelName: "  "}}},
		&healthResponse{},
		nil,
	)

	nodes, relations, err := testPlugin(t, baseURL).listInferenceEndpoints(t.Context(), nil)
	require.NoError(t, err)
	assert.Empty(t, nodes)
	assert.Empty(t, relations)
}

func TestListInferenceEndpointsMarksStatusUnknownWhenHealthUnreachable(t *testing.T) {
	baseURL := startLiteLLMModelInfo(t,
		modelInfoResponse{Data: []modelInfoEntry{{ModelName: "idp-model"}}},
		nil,
		map[string]int64{"idp-model": 1},
	)

	nodes, _, err := testPlugin(t, baseURL).listInferenceEndpoints(t.Context(), nil)
	require.NoError(t, err, "an unreachable health endpoint should not fail the whole sync")

	endpoints := nodePaths(nodes, pluginapi.NodeKindInferenceEndpoint)
	require.Contains(t, endpoints, "litellm/idp-model")
	assert.Equal(t, endpointStatusUnknown, endpoints["litellm/idp-model"][propertyKeyEndpointStatus])
}

func TestListInferenceEndpointsReportsUnreachableModelInfo(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	t.Cleanup(httpServer.Close)

	nodes, relations, err := testPlugin(t, httpServer.URL).listInferenceEndpoints(t.Context(), nil)
	require.Error(t, err)
	assert.Empty(t, nodes)
	assert.Empty(t, relations)
}

func TestModelInfoLiteLLMEndpointType(t *testing.T) {
	tests := []struct {
		name    string
		apiBase string
		want    string
	}{
		{"empty api_base is external", "", endpointTypeExternal},
		{"unparsable api_base is external", "://bad-url", endpointTypeExternal},
		{"localhost is internal", "http://localhost:4000", endpointTypeInternal},
		{"cluster-local .svc host is internal", "http://litellm.litellm.svc", endpointTypeInternal},
		{"cluster-local .svc.cluster.local host is internal", "http://litellm.litellm.svc.cluster.local:4000", endpointTypeInternal},
		{"private IP is internal", "http://10.0.0.5:8080", endpointTypeInternal},
		{"loopback IP is internal", "http://127.0.0.1:8080", endpointTypeInternal},
		{"public host is external", "https://api.anthropic.com", endpointTypeExternal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := modelInfoLiteLLM{APIBase: tt.apiBase}
			assert.Equal(t, tt.want, m.endpointType())
		})
	}
}

func TestModelInfoLiteLLMProvider(t *testing.T) {
	tests := []struct {
		name              string
		customLLMProvider string
		model             string
		want              string
	}{
		{"custom_llm_provider takes precedence", "anthropic", "openai/gpt-4o-mini", "anthropic"},
		{"derived from the model prefix", "", "anthropic/claude-3-5-sonnet-latest", "anthropic"},
		{"no provider when model has no prefix", "", "gpt-4o-mini", ""},
		{"empty when neither is set", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := modelInfoLiteLLM{CustomLLMProvider: tt.customLLMProvider, Model: tt.model}
			assert.Equal(t, tt.want, m.provider())
		})
	}
}
