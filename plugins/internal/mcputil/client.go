// Package mcputil provides a read-only client for the parts of the Model
// Context Protocol that Naira plugins need, so that every plugin cataloging MCP
// servers shares one implementation.

package mcputil

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/oauth2"
)

// clientName and clientVersion identify Naira to the MCP servers
const (
	clientName    = "naira-catalog"
	clientVersion = "v1"
)

type Target struct {
	Name        string
	Endpoint    string
	BearerToken string
}

// Capabilities reports which capability blocks the mcp server has
type Capabilities struct {
	Tools       bool
	Resources   bool
	Prompts     bool
	Logging     bool
	Completions bool
}

// ServerInfo holds the metadata a server reports when a session initializes.
type ServerInfo struct {
	ProtocolVersion string
	Name            string
	Title           string
	Version         string
	WebsiteURL      string
	Instructions    string
	Capabilities    Capabilities
}

// ToolAnnotations mirrors the behavioral hints a server may attach to a tool.
type ToolAnnotations struct {
	Title           string
	ReadOnlyHint    bool
	DestructiveHint *bool
	IdempotentHint  bool
	OpenWorldHint   *bool
}

// Tool describes a single tool exposed by an MCP server
type Tool struct {
	Name         string
	Title        string
	Description  string
	InputSchema  json.RawMessage
	OutputSchema json.RawMessage
	Annotations  *ToolAnnotations
}

// Inventory is everything one discovery pass learned about a server.
type Inventory struct {
	Server ServerInfo
	Tools  []Tool
}

// Opens a session to the target and reads its server metadata and tool list.
func Inspect(ctx context.Context, httpClient *http.Client, target Target) (Inventory, error) {
	transport, err := newTransport(httpClient, target)
	if err != nil {
		return Inventory{}, fmt.Errorf("building transport for %q: %w", target.Name, err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: clientName, Version: clientVersion}, nil)

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return Inventory{}, fmt.Errorf("initializing MCP session with %q: %w", target.Endpoint, err)
	}
	defer session.Close()

	inventory := Inventory{Server: serverInfo(session.InitializeResult())}

	if inventory.Server.Capabilities.Tools {
		tools, err := listTools(ctx, session)
		if err != nil {
			return inventory, fmt.Errorf("listing tools of %q: %w", target.Endpoint, err)
		}
		inventory.Tools = tools
	}

	return inventory, nil
}

func newTransport(httpClient *http.Client, target Target) (mcp.Transport, error) {
	if strings.TrimSpace(target.Endpoint) == "" {
		return nil, fmt.Errorf("endpoint is empty")
	}

	transport := &mcp.StreamableClientTransport{
		Endpoint:             target.Endpoint,
		HTTPClient:           httpClient,
		DisableStandaloneSSE: true,
	}
	if strings.TrimSpace(target.BearerToken) != "" {
		transport.OAuthHandler = staticToken{token: target.BearerToken}
	}

	return transport, nil
}

// TODO: Figure out better how to implement Auth.
// staticToken adapts a configured bearer token to the MCP SDK's OAuth handler
type staticToken struct {
	token string
}

func (s staticToken) TokenSource(context.Context) (oauth2.TokenSource, error) {
	return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: s.token}), nil
}

// Authorize is called after a 401 or 403
func (staticToken) Authorize(_ context.Context, _ *http.Request, resp *http.Response) error {
	defer resp.Body.Close()

	return fmt.Errorf("server rejected the configured bearer token: %s", resp.Status)
}

func serverInfo(result *mcp.InitializeResult) ServerInfo {
	if result == nil {
		return ServerInfo{}
	}

	info := ServerInfo{
		ProtocolVersion: result.ProtocolVersion,
		Instructions:    result.Instructions,
	}
	if result.ServerInfo != nil {
		info.Name = result.ServerInfo.Name
		info.Title = result.ServerInfo.Title
		info.Version = result.ServerInfo.Version
		info.WebsiteURL = result.ServerInfo.WebsiteURL
	}
	if result.Capabilities != nil {
		info.Capabilities = Capabilities{
			Tools:       result.Capabilities.Tools != nil,
			Resources:   result.Capabilities.Resources != nil,
			Prompts:     result.Capabilities.Prompts != nil,
			Logging:     result.Capabilities.Logging != nil,
			Completions: result.Capabilities.Completions != nil,
		}
	}

	return info
}

// listTools walks the paginated tools/list result
func listTools(ctx context.Context, session *mcp.ClientSession) ([]Tool, error) {
	var tools []Tool
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			return nil, fmt.Errorf("paging through tools: %w", err)
		}
		if tool == nil || strings.TrimSpace(tool.Name) == "" {
			continue
		}
		tools = append(tools, convertTool(tool))
	}

	return tools, nil
}

func convertTool(tool *mcp.Tool) Tool {
	converted := Tool{
		Name:         tool.Name,
		Title:        tool.Title,
		Description:  tool.Description,
		InputSchema:  encodeSchema(tool.InputSchema),
		OutputSchema: encodeSchema(tool.OutputSchema),
	}

	if tool.Annotations != nil {
		converted.Annotations = &ToolAnnotations{
			Title:           tool.Annotations.Title,
			ReadOnlyHint:    tool.Annotations.ReadOnlyHint,
			DestructiveHint: tool.Annotations.DestructiveHint,
			IdempotentHint:  tool.Annotations.IdempotentHint,
			OpenWorldHint:   tool.Annotations.OpenWorldHint,
		}
	}

	return converted
}

// encodeSchema converts a tool schema into the JSON stored on the node
func encodeSchema(schema any) json.RawMessage {
	if schema == nil {
		return nil
	}
	encoded, err := json.Marshal(schema)
	if err != nil || string(encoded) == "null" {
		return nil
	}

	return json.RawMessage(encoded)
}

// RedactEndpoint strips credentials and query parameters from an endpoint so it
// can be stored as catalog metadata. Tokens are commonly passed in either, and
// the catalog must not become a place where they leak.
func RedactEndpoint(endpoint string) string {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return ""
	}

	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""

	return parsed.String()
}
