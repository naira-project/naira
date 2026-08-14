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
			config:  config{PathPrefix: "litellm"},
			wantErr: "OPENAI_API_MODELS_BASE_URL is empty",
		},
		{
			name:    "missing node prefix",
			config:  config{BaseURL: "http://litellm.example.com"},
			wantErr: "PATH_PREFIX is empty",
		},
		{
			name:    "node prefix of only slashes",
			config:  config{BaseURL: "http://litellm.example.com", PathPrefix: "//"},
			wantErr: "PATH_PREFIX is empty",
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
		p, err := New(config{BaseURL: "http://litellm.example.com", PathPrefix: prefix}, testLogger())
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
		PathPrefix:  "vllm/",
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
			ID:         pluginapi.NodeID{Kind: pluginapi.NodeKindModel, Path: "vllm/gpt-4o"},
			Properties: pluginapi.PropertyMap{propertyKeyOwnedBy: "openai"},
		},
		{
			ID:         pluginapi.NodeID{Kind: pluginapi.NodeKindModel, Path: "vllm/claude-sonnet-5"},
			Properties: pluginapi.PropertyMap{propertyKeyOwnedBy: "anthropic"},
		},
		{
			ID:         pluginapi.NodeID{Kind: pluginapi.NodeKindModel, Path: "vllm/local-model"},
			Properties: pluginapi.PropertyMap{},
		},
	}, res.Nodes)
}

func TestCollectFlattensModelIDsToOneSegment(t *testing.T) {
	mockServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[
			{"id":"/models/qwen2.5-0.5b-instruct-q4_k_m.gguf"},
			{"id":"anthropic/claude-3.5-sonnet"},
			{"id":"/models/"},
			{"id":"/"}
		]}`)
	}))
	defer mockServer.Close()

	p, err := New(config{BaseURL: mockServer.URL, PathPrefix: "openai_llamacpp"}, testLogger())
	require.NoError(t, err)
	p.httpClient = mockServer.Client()

	res, err := p.Collect(context.Background())
	require.NoError(t, err)

	// Ids that collapse to nothing are skipped, and no path gains a second segment
	// - the UI reads the second-to-last segment as the source.
	assert.Equal(t, []pluginapi.NodeClaim{
		{
			ID:         pluginapi.NodeID{Kind: pluginapi.NodeKindModel, Path: "openai_llamacpp/qwen2.5-0.5b-instruct-q4_k_m.gguf"},
			Properties: pluginapi.PropertyMap{},
		},
		{
			ID:         pluginapi.NodeID{Kind: pluginapi.NodeKindModel, Path: "openai_llamacpp/claude-3.5-sonnet"},
			Properties: pluginapi.PropertyMap{},
		},
	}, res.Nodes)
}

func TestCollectSkipsModelsCollidingAfterFlattening(t *testing.T) {
	mockServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[
			{"id":"openai/gpt-4o","owned_by":"openai"},
			{"id":"azure/gpt-4o","owned_by":"azure"}
		]}`)
	}))
	defer mockServer.Close()

	p, err := New(config{BaseURL: mockServer.URL, PathPrefix: "litellm"}, testLogger())
	require.NoError(t, err)
	p.httpClient = mockServer.Client()

	res, err := p.Collect(context.Background())
	require.NoError(t, err)

	// Distinct ids sharing a basename collapse onto one path; the first claim wins.
	assert.Equal(t, []pluginapi.NodeClaim{
		{
			ID:         pluginapi.NodeID{Kind: pluginapi.NodeKindModel, Path: "litellm/gpt-4o"},
			Properties: pluginapi.PropertyMap{propertyKeyOwnedBy: "openai"},
		},
	}, res.Nodes)
}

func TestCollectPropagatesFetchError(t *testing.T) {
	mockServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mockServer.Close()

	p, err := New(config{BaseURL: mockServer.URL, PathPrefix: "litellm"}, testLogger())
	require.NoError(t, err)
	p.httpClient = mockServer.Client()

	_, err = p.Collect(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetching models from")
}

func testLogger() *log.Logger {
	return log.New(io.Discard, "", 0)
}
