// Command mcp-mock serves a mock MCP server for local development.
//
// It exists so the mcp_servers plugin has something to catalog in the kind
// cluster.
//
// # Environment Variables
//
//   - PORT (optional) - port to listen on; defaults to 8080.
//   - MCP_MOCK_SERVER_NAME (optional) - name the server reports during
//     initialization; defaults to "mock-knowledge-base". Set it to run several
//     instances with distinct identities.
//   - MCP_MOCK_TOKEN (optional) - when set, requests must carry
//     "Authorization: Bearer <token>". Unset means no authentication, which is
//     the default.
package main

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	defaultPort       = "8080"
	defaultServerName = "mock-knowledge-base"
)

func main() {
	logger := log.New(os.Stdout, "", log.LstdFlags)

	port := envOr("PORT", defaultPort)
	serverName := envOr("MCP_MOCK_SERVER_NAME", defaultServerName)
	token := os.Getenv("MCP_MOCK_TOKEN")

	mcpServer := newServer(serverName)
	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return mcpServer },
		nil,
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	mux.Handle("/mcp", requireToken(handler, token))
	mux.Handle("/mcp/", requireToken(handler, token))

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	logger.Printf("mock MCP server %q listening on :%s/mcp (auth required: %t)", serverName, port, token != "")
	if err := server.ListenAndServe(); err != nil {
		logger.Fatalf("failed to serve: %v", err)
	}
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}

// requireToken rejects unauthenticated requests when a token is configured
func requireToken(next http.Handler, token string) http.Handler {
	if token == "" {
		return next
	}

	expected := "Bearer " + token

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provided := r.Header.Get("Authorization")
		if subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func newServer(name string) *mcp.Server {
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:       name,
			Title:      "Mock Knowledge Base",
			Version:    "0.3.1",
			WebsiteURL: "https://example.internal/docs/mock-knowledge-base",
		},
		&mcp.ServerOptions{
			Instructions: "Mock server for Naira local development. Search internal documents, " +
				"file tickets, and summarize text. No call reaches a real system.",
		},
	)

	addTools(server)
	addResources(server)
	addPrompts(server)

	return server
}

func addTools(server *mcp.Server) {
	// Tool with a mix of required and optional parameters, and and ooutput schema.
	server.AddTool(&mcp.Tool{
		Name:        "search_documents",
		Title:       "Search Documents",
		Description: "Full-text search over the internal document store. Returns ranked excerpts.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":            map[string]any{"type": "string", "description": "Search expression."},
				"limit":            map[string]any{"type": "integer", "description": "Maximum results.", "default": 10},
				"include_archived": map[string]any{"type": "boolean", "default": false},
			},
			"required": []any{"query"},
		},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"matches": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           "Search",
			ReadOnlyHint:    true,
			IdempotentHint:  true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	}, textHandler("3 matching documents (mock)."))

	// Writing tool that reaches an external system.
	server.AddTool(&mcp.Tool{
		Name:        "create_ticket",
		Title:       "Create Ticket",
		Description: "File a ticket in the issue tracker.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title":    map[string]any{"type": "string"},
				"body":     map[string]any{"type": "string"},
				"priority": map[string]any{"type": "string", "enum": []any{"low", "normal", "high"}},
			},
			"required": []any{"title"},
		},
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			IdempotentHint:  false,
			OpenWorldHint:   boolPtr(true),
		},
	}, textHandler("Created ticket MOCK-42."))

	// Delete tool that declares itself destructive
	server.AddTool(&mcp.Tool{
		Name:        "delete_records",
		Title:       "Delete Records",
		Description: "Permanently delete records from the document store.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"record_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"confirm":    map[string]any{"type": "boolean"},
			},
			"required": []any{"record_ids", "confirm"},
		},
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			IdempotentHint:  false,
			OpenWorldHint:   boolPtr(false),
		},
	}, textHandler("Deleted 0 records (mock)."))

	// Tool with no annotations at all
	server.AddTool(&mcp.Tool{
		Name:        "summarize_text",
		Description: "Summarize a block of text.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{"type": "string"},
			},
			"required": []any{"text"},
		},
	}, textHandler("A short summary (mock)."))
}

func addResources(server *mcp.Server) {
	resources := []*mcp.Resource{
		{
			URI:         "mock://documents/incident-runbook",
			Name:        "incident-runbook",
			Title:       "Incident Runbook",
			Description: "Steps for handling a production incident.",
			MIMEType:    "text/markdown",
		},
		{
			URI:         "mock://documents/architecture-overview",
			Name:        "architecture-overview",
			Title:       "Architecture Overview",
			Description: "How the mock platform fits together.",
			MIMEType:    "text/markdown",
		},
	}

	for _, resource := range resources {
		server.AddResource(resource, func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{{
					URI:      req.Params.URI,
					MIMEType: "text/markdown",
					Text:     "# Mock document\n\nPlaceholder content.\n",
				}},
			}, nil
		})
	}
}

func addPrompts(server *mcp.Server) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "incident_summary",
		Title:       "Incident Summary",
		Description: "Summarize an incident for a status page.",
		Arguments: []*mcp.PromptArgument{
			{Name: "incident_id", Description: "Incident identifier.", Required: true},
			{Name: "audience", Description: "Who the summary is for."},
		},
	}, promptHandler("Summarize the incident for the given audience."))

	server.AddPrompt(&mcp.Prompt{
		Name:        "release_notes",
		Title:       "Release Notes",
		Description: "Draft release notes from a list of changes.",
		Arguments: []*mcp.PromptArgument{
			{Name: "version", Required: true},
		},
	}, promptHandler("Draft release notes for the given version."))
}

func boolPtr(value bool) *bool { return &value }

func textHandler(text string) mcp.ToolHandler {
	return func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: text}},
		}, nil
	}
}

func promptHandler(text string) mcp.PromptHandler {
	return func(context.Context, *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{
			Messages: []*mcp.PromptMessage{{
				Role:    "user",
				Content: &mcp.TextContent{Text: text},
			}},
		}, nil
	}
}
