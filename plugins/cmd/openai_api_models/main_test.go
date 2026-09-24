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
			name: "missing base URL",
			config: config{
				PathPrefix: "litellm",
			},
			wantErr: "OPENAI_API_MODELS_BASE_URL is empty",
		},
		{
			name: "missing node prefix",
			config: config{
				BaseURL: "http://litellm.example.com",
			},
			wantErr: "PATH_PREFIX is empty",
		},
		{
			name: "node prefix of only slashes",
			config: config{
				BaseURL:    "http://litellm.example.com",
				PathPrefix: "//",
			},
			wantErr: `PATH_PREFIX must not contain "/"`,
		},
		{
			name: "node prefix with trailing slash",
			config: config{
				BaseURL:    "http://litellm.example.com",
				PathPrefix: "litellm/",
			},
			wantErr: `PATH_PREFIX must not contain "/"`,
		},
		{
			name: "node prefix with multiple segments",
			config: config{
				BaseURL:    "http://litellm.example.com",
				PathPrefix: "models/litellm",
			},
			wantErr: `PATH_PREFIX must not contain "/"`,
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

func TestNewTrimsSurroundingSpaceFromNodePrefix(t *testing.T) {
	// Whitespace is invisible in a manifest, so it is trimmed rather than rejected.
	prefixTests := []string{
		"litellm",
		" litellm ",
		"\tlitellm\n",
	}
	for _, prefix := range prefixTests {
		p, err := New(config{
			BaseURL:    "http://litellm.example.com",
			PathPrefix: prefix,
		}, testLogger())
		require.NoError(t, err)
		assert.Equal(t, "litellm", p.nodePrefix, "prefix %q", prefix)
	}
}

func TestCollect(t *testing.T) {
	mockServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/models", r.URL.Path)
		assert.Equal(t, "Bearer test-api-key", r.Header.Get("Authorization"))
		fmt.Fprint(w, `{
            "data":[
                {
                    "id": "gpt-4o",
                    "owned_by": "openai"
                },
                {
                    "id": "claude-sonnet-5",
                    "owned_by": "anthropic"
                },
                {
                    "id": "local-model"
                },
                {
                    "id": "gpt-4o",
                    "owned_by": "openai"
                },
                {
                    "id": "many-extra-fields",
                    "test-text": "aaa",
                    "test-number": 123,
                    "test-object": {"a": 1, "b": 2},
                    "test-array": [1, 2, 3]
                },
                {
                    "id": "  "
                }
            ]
        }`)
	}))
	defer mockServer.Close()

	p, err := New(config{
		BaseURL:     mockServer.URL,
		PathPrefix:  "vllm",
		APIKey:      "test-api-key",
		HTTPTimeout: 5 * time.Second,
	}, testLogger())
	require.NoError(t, err)
	p.httpClient = mockServer.Client()

	res, err := p.Collect(context.Background())
	require.NoError(t, err)

	assert.Empty(t, res.Relations)
	assert.Equal(t, []pluginapi.NodeClaim{
		{ID: nodeID("model", "vllm/gpt-4o"),
			Properties: pluginapi.PropertyMap{
				"owned_by": "openai"},
		},
		{ID: nodeID("model", "vllm/claude-sonnet-5"),
			Properties: pluginapi.PropertyMap{
				"owned_by": "anthropic"},
		},
		{ID: nodeID("model", "vllm/local-model")},
		{ID: nodeID("model", "vllm/many-extra-fields"),
			Properties: pluginapi.PropertyMap{
				"test-text":     "aaa",
				"test-number":   "123",
				"test-object.a": "1",
				"test-object.b": "2",
				"test-array":    "[1, 2, 3]",
			},
		},
	}, res.Nodes)
}

func TestCollectPropagatesFetchError(t *testing.T) {
	mockServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mockServer.Close()

	p, err := New(config{
		BaseURL:    mockServer.URL,
		PathPrefix: "litellm",
	}, testLogger())
	require.NoError(t, err)
	p.httpClient = mockServer.Client()

	_, err = p.Collect(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetching models from")
}

func testLogger() *log.Logger {
	return log.New(io.Discard, "", 0)
}

func nodeID(kind, path string) pluginapi.NodeID {
	return pluginapi.NodeID{Kind: kind, Path: path}
}
