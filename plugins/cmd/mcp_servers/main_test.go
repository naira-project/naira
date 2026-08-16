package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptr[T any](value T) *T { return &value }

func testLogger() *log.Logger { return log.New(io.Discard, "", 0) }

// startServer hosts an MCP server over streamable HTTP and returns its URL.
func startServer(t *testing.T, configure func(*mcp.Server)) string {
	t.Helper()

	server := mcp.NewServer(
		&mcp.Implementation{Name: "reported-name", Title: "Reported", Version: "4.5.6"},
		&mcp.ServerOptions{Instructions: "Test instructions."},
	)
	configure(server)

	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	httpServer := httptest.NewServer(handler)
	t.Cleanup(httpServer.Close)

	return httpServer.URL
}

func addTool(server *mcp.Server, tool *mcp.Tool) {
	server.AddTool(tool, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{}, nil
	})
}

// nodesByKind indexes a collect response by node kind and path.
func nodesByKind(response pluginapi.CollectResponse, kind string) map[string]pluginapi.PropertyMap {
	indexed := make(map[string]pluginapi.PropertyMap)
	for _, node := range response.Nodes {
		if node.ID.Kind == kind {
			indexed[node.ID.Path] = node.Properties
		}
	}

	return indexed
}

func TestNewValidatesConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  config
		wantErr string
	}{
		{
			name:    "missing node prefix",
			config:  config{Endpoints: "a=http://example.com/mcp"},
			wantErr: "PATH_PREFIX is empty",
		},
		{
			name:    "node prefix of only slashes",
			config:  config{PathPrefix: "//", Endpoints: "a=http://example.com/mcp"},
			wantErr: "PATH_PREFIX is empty",
		},
		{
			name:    "no endpoints",
			config:  config{PathPrefix: "platform"},
			wantErr: "no endpoints configured",
		},
		{
			name:    "endpoint without name",
			config:  config{PathPrefix: "platform", Endpoints: "http://example.com/mcp"},
			wantErr: "must be in name=url format",
		},
		{
			name:    "endpoint with empty url",
			config:  config{PathPrefix: "platform", Endpoints: "a="},
			wantErr: "name and url must not be empty",
		},
		{
			name:    "endpoint name with path separator",
			config:  config{PathPrefix: "platform", Endpoints: "a/b=http://example.com/mcp"},
			wantErr: "must not contain '/'",
		},
		{
			name:    "duplicate endpoint names",
			config:  config{PathPrefix: "platform", Endpoints: "a=http://one/mcp,a=http://two/mcp"},
			wantErr: `duplicate endpoint name "a"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := New(test.config, testLogger())
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.wantErr)
		})
	}
}

func TestCollectEmitsServerAndTools(t *testing.T) {
	endpoint := startServer(t, func(server *mcp.Server) {
		addTool(server, &mcp.Tool{
			Name:        "search_code",
			Title:       "Search Code",
			Description: "Search the codebase.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
					"limit": map[string]any{"type": "integer"},
				},
				"required": []any{"query"},
			},
			Annotations: &mcp.ToolAnnotations{
				ReadOnlyHint:    true,
				DestructiveHint: ptr(false),
				OpenWorldHint:   ptr(true),
			},
		})
		server.AddPrompt(&mcp.Prompt{Name: "summarize"}, func(context.Context, *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			return &mcp.GetPromptResult{}, nil
		})
	})

	plugin, err := New(config{PathPrefix: "platform/", Endpoints: "github=" + endpoint, HTTPTimeout: 5 * time.Second}, testLogger())
	require.NoError(t, err)

	response, err := plugin.Collect(t.Context())
	require.NoError(t, err)

	servers := nodesByKind(response, pluginapi.NodeKindMCPServer)
	require.Contains(t, servers, "platform/github", "server path uses the configured name, not the reported one")
	server := servers["platform/github"]

	assert.Equal(t, "true", server[propertyKeyReachable])
	assert.Equal(t, "reported-name", server[propertyKeyServerName])
	assert.Equal(t, "Reported", server[propertyKeyServerTitle])
	assert.Equal(t, "4.5.6", server[propertyKeyServerVersion])
	assert.Equal(t, "Test instructions.", server[propertyKeyInstructions])
	assert.Equal(t, "none", server[propertyKeyAuthType])
	assert.Equal(t, "true", server[propertyKeyCapabilityTools])
	assert.Equal(t, "true", server[propertyKeyCapabilityPrompts])
	assert.Equal(t, "false", server[propertyKeyCapabilityResources])
	assert.NotEmpty(t, server[propertyKeyProtocolVersion])
	assert.NotContains(t, server, propertyKeyError)

	tools := nodesByKind(response, pluginapi.NodeKindMCPTool)
	require.Contains(t, tools, "platform/github/search_code")
	tool := tools["platform/github/search_code"]

	assert.Equal(t, "Search Code", tool[propertyKeyTitle])
	assert.Equal(t, "Search the codebase.", tool[propertyKeyDescription])
	assert.JSONEq(t, `{"type":"object","properties":{"query":{"type":"string"},"limit":{"type":"integer"}},"required":["query"]}`, tool[propertyKeyInputSchema])
	assert.Equal(t, "true", tool[propertyKeyReadOnlyHint])
	assert.Equal(t, "false", tool[propertyKeyDestructiveHint])
	assert.Equal(t, "true", tool[propertyKeyOpenWorldHint])
	assert.Equal(t, "false", tool[propertyKeyIdempotentHint])

	require.Len(t, response.Relations, 1)
	relation := response.Relations[0]
	assert.Equal(t, pluginapi.RelationKindExposes, relation.Kind)
	assert.Equal(t, pluginapi.NodeID{Kind: pluginapi.NodeKindMCPServer, Path: "platform/github"}, relation.From)
	assert.Equal(t, pluginapi.NodeID{Kind: pluginapi.NodeKindMCPTool, Path: "platform/github/search_code"}, relation.To)
}

func TestCollectOmitsUndeclaredAnnotationHints(t *testing.T) {
	endpoint := startServer(t, func(server *mcp.Server) {
		addTool(server, &mcp.Tool{Name: "tool", InputSchema: map[string]any{"type": "object"}})
	})

	plugin, err := New(config{PathPrefix: "platform", Endpoints: "srv=" + endpoint}, testLogger())
	require.NoError(t, err)

	response, err := plugin.Collect(t.Context())
	require.NoError(t, err)

	tool := nodesByKind(response, pluginapi.NodeKindMCPTool)["platform/srv/tool"]
	require.NotNil(t, tool)
	// The protocol defaults these two hints to true, so an absent value must not
	// be recorded as a declared false.
	assert.NotContains(t, tool, propertyKeyDestructiveHint)
	assert.NotContains(t, tool, propertyKeyOpenWorldHint)
}

func TestCollectKeepsUnreachableServerVisible(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(httpServer.Close)

	plugin, err := New(config{PathPrefix: "platform", Endpoints: "broken=" + httpServer.URL}, testLogger())
	require.NoError(t, err)

	response, err := plugin.Collect(t.Context())
	require.NoError(t, err, "one unreachable server must not fail the whole collection")

	servers := nodesByKind(response, pluginapi.NodeKindMCPServer)
	require.Contains(t, servers, "platform/broken")
	server := servers["platform/broken"]

	assert.Equal(t, "false", server[propertyKeyReachable])
	assert.NotEmpty(t, server[propertyKeyError])
	assert.NotContains(t, server, propertyKeyCapabilityTools, "capabilities are unknown when the session never opened")
	assert.Empty(t, nodesByKind(response, pluginapi.NodeKindMCPTool))
}

func TestCollectContinuesAfterUnreachableServer(t *testing.T) {
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(broken.Close)

	healthy := startServer(t, func(server *mcp.Server) {
		addTool(server, &mcp.Tool{Name: "tool", InputSchema: map[string]any{"type": "object"}})
	})

	plugin, err := New(config{PathPrefix: "platform", Endpoints: "broken=" + broken.URL + ",healthy=" + healthy}, testLogger())
	require.NoError(t, err)

	response, err := plugin.Collect(t.Context())
	require.NoError(t, err)

	servers := nodesByKind(response, pluginapi.NodeKindMCPServer)
	assert.Equal(t, "false", servers["platform/broken"][propertyKeyReachable])
	assert.Equal(t, "true", servers["platform/healthy"][propertyKeyReachable])
	assert.Contains(t, nodesByKind(response, pluginapi.NodeKindMCPTool), "platform/healthy/tool")
}

func TestCollectRedactsEndpointCredentials(t *testing.T) {
	plugin, err := New(config{
		PathPrefix: "platform",
		Endpoints:  "secret=https://user:pass@mcp.example.com/mcp?token=abc",
	}, testLogger())
	require.NoError(t, err)
	// A short timeout keeps the unreachable-host case fast; the node is emitted
	// either way and the endpoint property is what matters here.
	plugin.httpClient = &http.Client{Timeout: 100 * time.Millisecond}

	response, err := plugin.Collect(t.Context())
	require.NoError(t, err)

	server := nodesByKind(response, pluginapi.NodeKindMCPServer)["platform/secret"]
	require.NotNil(t, server)
	assert.Equal(t, "https://mcp.example.com/mcp", server[propertyKeyEndpoint])
	assert.NotContains(t, server[propertyKeyEndpoint], "pass")
	assert.NotContains(t, server[propertyKeyEndpoint], "abc")
}

func TestBearerTokenAppliesToEveryEndpoint(t *testing.T) {
	plugin, err := New(config{
		PathPrefix:  "platform",
		Endpoints:   "my-server=http://one/mcp,other=http://two/mcp",
		BearerToken: "shared",
	}, testLogger())
	require.NoError(t, err)

	require.Len(t, plugin.targets, 2)
	for _, target := range plugin.targets {
		assert.Equal(t, "shared", target.BearerToken, "endpoint %q", target.Name)
	}
}

func TestCollectTruncatesOverlongText(t *testing.T) {
	longDescription := ""
	for range maxTextLength + 100 {
		longDescription += "é"
	}

	endpoint := startServer(t, func(server *mcp.Server) {
		addTool(server, &mcp.Tool{
			Name:        "tool",
			Description: longDescription,
			InputSchema: map[string]any{"type": "object"},
		})
	})

	plugin, err := New(config{PathPrefix: "platform", Endpoints: "srv=" + endpoint}, testLogger())
	require.NoError(t, err)

	response, err := plugin.Collect(t.Context())
	require.NoError(t, err)

	description := nodesByKind(response, pluginapi.NodeKindMCPTool)["platform/srv/tool"][propertyKeyDescription]
	assert.Equal(t, maxTextLength+1, len([]rune(description)), "truncation must cut on a rune boundary")
	assert.True(t, utf8ValidString(description))
}

func utf8ValidString(s string) bool {
	for _, r := range s {
		if r == '�' {
			return false
		}
	}

	return true
}
