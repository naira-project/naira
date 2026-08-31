package mcputil_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/naira-project/naira/plugins/internal/mcputil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptr[T any](value T) *T { return &value }

// startServer hosts an MCP server over streamable HTTP and returns its URL.
func startServer(t *testing.T, configure func(*mcp.Server)) string {
	t.Helper()

	server := mcp.NewServer(
		&mcp.Implementation{Name: "test-server", Title: "Test Server", Version: "1.2.3"},
		&mcp.ServerOptions{Instructions: "Use this server for tests."},
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

func TestInspectCollectsServerMetadata(t *testing.T) {
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
				Title:           "Search",
				ReadOnlyHint:    true,
				IdempotentHint:  true,
				DestructiveHint: ptr(false),
				OpenWorldHint:   ptr(true),
			},
		})
	})

	inventory, err := mcputil.Inspect(t.Context(), http.DefaultClient, mcputil.Target{
		Name:     "test",
		Endpoint: endpoint,
	})
	require.NoError(t, err)

	assert.Equal(t, "test-server", inventory.Server.Name)
	assert.Equal(t, "Test Server", inventory.Server.Title)
	assert.Equal(t, "1.2.3", inventory.Server.Version)
	assert.Equal(t, "Use this server for tests.", inventory.Server.Instructions)
	assert.NotEmpty(t, inventory.Server.ProtocolVersion, "protocol version drives the MCP revision tag")
	assert.True(t, inventory.Server.Capabilities.Tools)

	require.Len(t, inventory.Tools, 1)
	tool := inventory.Tools[0]
	assert.Equal(t, "search_code", tool.Name)
	assert.Equal(t, "Search Code", tool.Title)
	assert.Equal(t, "Search the codebase.", tool.Description)
	assert.JSONEq(t, `{"type":"object","properties":{"query":{"type":"string"},"limit":{"type":"integer"}},"required":["query"]}`, string(tool.InputSchema))

	require.NotNil(t, tool.Annotations)
	assert.Equal(t, "Search", tool.Annotations.Title)
	assert.True(t, tool.Annotations.ReadOnlyHint)
	assert.True(t, tool.Annotations.IdempotentHint)
	assert.Equal(t, ptr(false), tool.Annotations.DestructiveHint)
	assert.Equal(t, ptr(true), tool.Annotations.OpenWorldHint)
}

func TestInspectSkipsToolsWhenNotAdvertised(t *testing.T) {
	endpoint := startServer(t, func(server *mcp.Server) {
		server.AddPrompt(&mcp.Prompt{Name: "summarize"}, func(context.Context, *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			return &mcp.GetPromptResult{}, nil
		})
	})

	inventory, err := mcputil.Inspect(t.Context(), http.DefaultClient, mcputil.Target{Name: "test", Endpoint: endpoint})
	require.NoError(t, err)

	assert.Empty(t, inventory.Tools)
	assert.True(t, inventory.Server.Capabilities.Prompts)
	assert.False(t, inventory.Server.Capabilities.Tools)
}

func TestInspectSendsBearerToken(t *testing.T) {
	var gotAuthorization string
	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "1"}, nil)
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return mcpServer }, nil)

	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthorization = r.Header.Get("Authorization")
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(httpServer.Close)

	_, err := mcputil.Inspect(t.Context(), http.DefaultClient, mcputil.Target{
		Name:        "test",
		Endpoint:    httpServer.URL,
		BearerToken: "s3cret",
	})
	require.NoError(t, err)

	assert.Equal(t, "Bearer s3cret", gotAuthorization)
}

func TestInspectUnreachableServer(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	t.Cleanup(httpServer.Close)

	_, err := mcputil.Inspect(t.Context(), http.DefaultClient, mcputil.Target{Name: "test", Endpoint: httpServer.URL})
	assert.Error(t, err)
}

func TestRedactEndpoint(t *testing.T) {
	tests := map[string]string{
		"https://user:pass@mcp.example.com/mcp": "https://mcp.example.com/mcp",
		"https://mcp.example.com/mcp?token=abc": "https://mcp.example.com/mcp",
		"http://tickets.svc:8080/mcp":           "http://tickets.svc:8080/mcp",
		"https://mcp.example.com/mcp#frag":      "https://mcp.example.com/mcp",
	}

	for endpoint, expected := range tests {
		assert.Equal(t, expected, mcputil.RedactEndpoint(endpoint), "endpoint %q", endpoint)
	}
}
