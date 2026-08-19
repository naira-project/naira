package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startLiteLLM serves the MCP registry at /v1/mcp/server and proxies each named
// server at /<name>/mcp, mirroring how LiteLLM exposes them.
func startLiteLLM(t *testing.T, registry []map[string]any, mcpServers map[string]*mcp.Server) string {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/mcp/server", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(registry))
	})

	for name, server := range mcpServers {
		handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
		mux.Handle("/"+name+"/mcp", handler)
		mux.Handle("/"+name+"/mcp/", handler)
	}

	httpServer := httptest.NewServer(mux)
	t.Cleanup(httpServer.Close)

	return httpServer.URL
}

func mcpServerWithTool(name, toolName string) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: name, Version: "1.0.0"}, nil)
	server.AddTool(
		&mcp.Tool{Name: toolName, Description: "A tool.", InputSchema: map[string]any{"type": "object"}},
		func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{}, nil
		},
	)

	return server
}

func testPlugin(t *testing.T, baseURL string) *Plugin {
	t.Helper()

	return New(config{PathPrefix: "litellm", BaseURL: baseURL, APIKey: "sk-test"}, log.New(io.Discard, "", 0))
}

func nodePaths(nodes []pluginapi.NodeClaim, kind string) map[string]pluginapi.PropertyMap {
	indexed := make(map[string]pluginapi.PropertyMap)
	for _, node := range nodes {
		if node.ID.Kind == kind {
			indexed[node.ID.Path] = node.Properties
		}
	}

	return indexed
}

func TestCollectMCPServersEmitsOneNodePerServer(t *testing.T) {
	baseURL := startLiteLLM(t,
		[]map[string]any{
			{
				"server_id":         "srv-1",
				"server_name":       "github",
				"alias":             "GitHub",
				"description":       "Code search.",
				"auth_type":         "oauth2",
				"status":            "healthy",
				"teams":             []string{"platform", "sre"},
				"mcp_access_groups": []string{"internal"},
			},
			{"server_id": "srv-2", "server_name": "tickets"},
		},
		map[string]*mcp.Server{
			"github":  mcpServerWithTool("github-mcp", "search_code"),
			"tickets": mcpServerWithTool("tickets-mcp", "create_ticket"),
		},
	)

	nodes, relations := testPlugin(t, baseURL).collectMCPServers(t.Context())

	servers := nodePaths(nodes, pluginapi.NodeKindMCPServer)
	require.Contains(t, servers, "litellm/github", "each proxied server gets its own node, not one gateway node")
	require.Contains(t, servers, "litellm/tickets")

	github := servers["litellm/github"]
	assert.Equal(t, "true", github["reachable"])
	assert.Equal(t, "github-mcp", github["server_name"], "protocol metadata comes from the server itself")
	assert.Equal(t, "Code search.", github["description"], "registry metadata the protocol cannot express")
	assert.Equal(t, "GitHub", github["litellm_alias"])
	assert.Equal(t, "srv-1", github["litellm_server_id"])
	assert.Equal(t, "oauth2", github["litellm_upstream_auth_type"])
	assert.Equal(t, "healthy", github["litellm_status"])
	assert.Equal(t, "platform,sre", github["litellm_teams"])
	assert.Equal(t, "internal", github["litellm_access_groups"])
	assert.Equal(t, "bearer", github["auth_type"], "auth_type describes how Naira reaches LiteLLM")

	assert.NotContains(t, servers["litellm/tickets"], "litellm_teams", "absent registry fields stay absent")

	tools := nodePaths(nodes, pluginapi.NodeKindMCPTool)
	assert.Contains(t, tools, "litellm/github/search_code")
	assert.Contains(t, tools, "litellm/tickets/create_ticket")

	assert.Len(t, relations, 2)
	for _, relation := range relations {
		assert.Equal(t, pluginapi.RelationKindExposes, relation.Kind)
	}
}

func TestCollectMCPServersSkipsUnusableNames(t *testing.T) {
	baseURL := startLiteLLM(t,
		[]map[string]any{
			{"server_id": "srv-1", "server_name": ""},
			{"server_id": "srv-2", "server_name": "a/b"},
		},
		nil,
	)

	nodes, relations := testPlugin(t, baseURL).collectMCPServers(t.Context())

	assert.Empty(t, nodes, "a name that cannot form a URL segment cannot be reached or addressed")
	assert.Empty(t, relations)
}

func TestCollectMCPServersTolerantOfMissingGateway(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	t.Cleanup(httpServer.Close)

	nodes, relations := testPlugin(t, httpServer.URL).collectMCPServers(t.Context())

	assert.Empty(t, nodes, "a LiteLLM without the MCP gateway must not fail the collection")
	assert.Empty(t, relations)
}

func TestCollectMCPServersKeepsUnreachableServerVisible(t *testing.T) {
	// Registered in the registry but not served, so the session cannot open.
	baseURL := startLiteLLM(t, []map[string]any{{"server_id": "srv-1", "server_name": "ghost"}}, nil)

	nodes, relations := testPlugin(t, baseURL).collectMCPServers(t.Context())

	servers := nodePaths(nodes, pluginapi.NodeKindMCPServer)
	require.Contains(t, servers, "litellm/ghost")
	assert.Equal(t, "false", servers["litellm/ghost"]["reachable"])
	assert.NotEmpty(t, servers["litellm/ghost"]["error"])
	assert.Empty(t, relations)
}
