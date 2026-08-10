package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewValidatesConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  config
		wantErr string
	}{
		{
			name:    "missing base URL",
			config:  config{NodePrefix: "litellm"},
			wantErr: "OPENAI_API_MODELS_BASE_URL is empty",
		},
		{
			name:    "missing node prefix",
			config:  config{BaseURL: "http://litellm.example.com"},
			wantErr: "OPENAI_API_MODELS_NODE_PREFIX is empty",
		},
		{
			name:    "node prefix of only slashes",
			config:  config{BaseURL: "http://litellm.example.com", NodePrefix: "//"},
			wantErr: "OPENAI_API_MODELS_NODE_PREFIX is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.config, testLogger())
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestNewNormalizesNodePrefix(t *testing.T) {
	for _, prefix := range []string{"litellm", "litellm/", "/litellm/", " litellm "} {
		p, err := New(config{BaseURL: "http://litellm.example.com", NodePrefix: prefix}, testLogger())
		require.NoError(t, err)
		assert.Equal(t, "litellm", p.nodePrefix, "prefix %q", prefix)
	}
}

func TestCollect(t *testing.T) {
	const testAPIKey = "sk-test"

	mockServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/models", r.URL.Path)
		assert.Equal(t, "Bearer "+testAPIKey, r.Header.Get("Authorization"))
		fmt.Fprint(w, `{"data":[
			{"id":"gpt-4o","owned_by":"openai"},
			{"id":"claude-sonnet-5","owned_by":"anthropic"},
			{"id":"local-model"},
			{"id":"gpt-4o","owned_by":"openai"},
			{"id":"  "}
		]}`)
	}))
	defer mockServer.Close()

	p, err := New(config{
		BaseURL:     mockServer.URL,
		NodePrefix:  "thalamus/",
		APIKey:      testAPIKey,
		HTTPTimeout: 5 * time.Second,
	}, testLogger())
	require.NoError(t, err)
	p.httpClient = mockServer.Client()

	res, err := p.Collect(context.Background())
	require.NoError(t, err)

	assert.Empty(t, res.Relations)
	assert.Equal(t, []pluginapi.NodeClaim{
		{
			ID:         pluginapi.NodeID{Kind: pluginapi.NodeKindModel, Path: "thalamus/gpt-4o"},
			Properties: pluginapi.PropertyMap{propertyKeyOwnedBy: "openai"},
		},
		{
			ID:         pluginapi.NodeID{Kind: pluginapi.NodeKindModel, Path: "thalamus/claude-sonnet-5"},
			Properties: pluginapi.PropertyMap{propertyKeyOwnedBy: "anthropic"},
		},
		{
			ID:         pluginapi.NodeID{Kind: pluginapi.NodeKindModel, Path: "thalamus/local-model"},
			Properties: pluginapi.PropertyMap{},
		},
	}, res.Nodes)
}

func TestCollectPropagatesFetchError(t *testing.T) {
	mockServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mockServer.Close()

	p, err := New(config{BaseURL: mockServer.URL, NodePrefix: "litellm"}, testLogger())
	require.NoError(t, err)
	p.httpClient = mockServer.Client()

	_, err = p.Collect(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetching models from")
}

func testLogger() *log.Logger {
	return log.New(io.Discard, "", 0)
}
