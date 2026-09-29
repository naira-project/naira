package mcputil_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/naira-project/naira/plugins/internal/mcputil"
	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nodesByKind indexes a set of claims by node path.
func nodesByKind(nodes []pluginapi.NodeClaim, kind string) map[string]pluginapi.PropertyMap {
	indexed := make(map[string]pluginapi.PropertyMap)
	for _, node := range nodes {
		if node.ID.Kind == kind {
			indexed[node.ID.Path] = node.Properties
		}
	}

	return indexed
}

func inventoryFixture() mcputil.Inventory {
	return mcputil.Inventory{
		Server: mcputil.ServerInfo{
			ProtocolVersion: "2025-06-18",
			Name:            "reported-name",
			Title:           "Reported",
			Version:         "4.5.6",
			Instructions:    "Test instructions.",
			Capabilities:    mcputil.Capabilities{Tools: true, Prompts: true},
		},
		Tools: []mcputil.Tool{{
			Name:        "search_documents",
			Title:       "Search Documents",
			Description: "Search the store.",
			InputSchema: json.RawMessage(`{"type":"object"}`),
			Annotations: &mcputil.ToolAnnotations{
				ReadOnlyHint:    true,
				DestructiveHint: ptr(false),
			},
		}},
	}
}

func TestGraphBuildsServerAndTools(t *testing.T) {
	target := mcputil.Target{Name: "kb", Endpoint: "https://user:pw@mcp.example.com/mcp?token=abc"}

	nodes, relations := mcputil.Graph("platform", target, inventoryFixture(), nil, nil)

	servers := nodesByKind(nodes, pluginapi.NodeKindMCPServer)
	require.Contains(t, servers, "platform/kb")
	server := servers["platform/kb"]

	assert.Equal(t, "true", server["reachable"])
	assert.Equal(t, "reported-name", server["server_name"])
	assert.Equal(t, "Reported", server["server_title"])
	assert.Equal(t, "4.5.6", server["server_version"])
	assert.Equal(t, "2025-06-18", server["protocol_version"])
	assert.Equal(t, "Test instructions.", server["instructions"])
	assert.Equal(t, "none", server["auth_type"])
	assert.Equal(t, "true", server["capability_tools"])
	assert.Equal(t, "false", server["capability_resources"])
	assert.Equal(t, "https://mcp.example.com/mcp", server["endpoint"], "credentials must not reach the catalog")
	assert.NotContains(t, server, "error")

	tools := nodesByKind(nodes, pluginapi.NodeKindMCPTool)
	require.Contains(t, tools, "platform/kb/search_documents")
	tool := tools["platform/kb/search_documents"]

	assert.Equal(t, "Search Documents", tool["title"])
	assert.Equal(t, "Search the store.", tool["description"])
	assert.JSONEq(t, `{"type":"object"}`, tool["input_schema"])
	assert.Equal(t, "true", tool["readonly_hint"])
	assert.Equal(t, "false", tool["destructive_hint"])
	assert.NotContains(t, tool, "open_world_hint", "an undeclared hint must not become a declared false")

	require.Len(t, relations, 1)
	assert.Equal(t, pluginapi.RelationKindExposes, relations[0].Kind)
	assert.Equal(t, "platform/kb", relations[0].From.Path)
	assert.Equal(t, "platform/kb/search_documents", relations[0].To.Path)
}

func TestGraphKeepsUnreachableServerVisible(t *testing.T) {
	target := mcputil.Target{Name: "kb", Endpoint: "http://mcp.example.com/mcp", BearerToken: "s3cret"}

	nodes, relations := mcputil.Graph("platform", target, mcputil.Inventory{}, errors.New("boom"), nil)

	require.Len(t, nodes, 1, "a failed inspection must not emit tool nodes")
	assert.Empty(t, relations)

	server := nodes[0].Properties
	assert.Equal(t, "false", server["reachable"])
	assert.Equal(t, "boom", server["error"])
	assert.Equal(t, "bearer", server["auth_type"])
	assert.NotContains(t, server, "capability_tools", "capabilities are unknown when the session never opened")
}

func TestGraphMergesExtraProperties(t *testing.T) {
	target := mcputil.Target{Name: "kb", Endpoint: "http://mcp.example.com/mcp"}
	extra := pluginapi.PropertyMap{"litellm_teams": "platform", "litellm_status": "healthy", "ignored": ""}

	nodes, _ := mcputil.Graph("litellm", target, inventoryFixture(), nil, extra)

	server := nodesByKind(nodes, pluginapi.NodeKindMCPServer)["litellm/kb"]
	require.NotNil(t, server)
	assert.Equal(t, "platform", server["litellm_teams"])
	assert.Equal(t, "healthy", server["litellm_status"])
	assert.NotContains(t, server, "ignored", "empty extras must not create properties")
	assert.Equal(t, "reported-name", server["server_name"], "extras must not displace protocol metadata")
}
