// mcp_servers plugin that catalogs MCP servers and the tools they expose by
// talking to each configured server directly.
//
// # Environment Variables
//
//   - MCP_ENDPOINTS - MANDATORY - comma-separated list of MCP servers in
//     "name=url"
//
//   - PATH_PREFIX - MANDATORY - prefix for the emitted Node paths
//
//   - MCP_BEARER_TOKEN (optional) - bearer token sent to every configured
//     server. Run a second plugin instance for servers needing a different
//     credential.
//
//   - MCP_HTTP_TIMEOUT (optional) - per-server HTTP timeout; defaults to 10s.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/naira-project/naira/plugins/internal/mcputil"
	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/naira-project/naira/plugins/pkg/pluginmain"
)

// Property keys for mcp_server nodes.
const (
	propertyKeyProtocolVersion = "protocol_version"
	propertyKeyServerName      = "server_name"
	propertyKeyServerTitle     = "server_title"
	propertyKeyServerVersion   = "server_version"
	propertyKeyWebsiteURL      = "website_url"
	propertyKeyInstructions    = "instructions"
	propertyKeyEndpoint        = "endpoint"
	propertyKeyAuthType        = "auth_type"
	propertyKeyReachable       = "reachable"
	propertyKeyError           = "error"
)

// Property keys for advertised server capabilities.
const (
	propertyKeyCapabilityTools       = "capability_tools"
	propertyKeyCapabilityResources   = "capability_resources"
	propertyKeyCapabilityPrompts     = "capability_prompts"
	propertyKeyCapabilityLogging     = "capability_logging"
	propertyKeyCapabilityCompletions = "capability_completions"
)

// Property keys for mcp_tool nodes.
const (
	propertyKeyTitle           = "title"
	propertyKeyDescription     = "description"
	propertyKeyInputSchema     = "input_schema"
	propertyKeyOutputSchema    = "output_schema"
	propertyKeyAnnotationTitle = "annotation_title"
	propertyKeyReadOnlyHint    = "readonly_hint"
	propertyKeyDestructiveHint = "destructive_hint"
	propertyKeyIdempotentHint  = "idempotent_hint"
	propertyKeyOpenWorldHint   = "open_world_hint"
)

// Values for the auth_type
const (
	authTypeNone   = "none"
	authTypeBearer = "bearer"
)

// maxTextLength for fields
const maxTextLength = 2000

type config struct {
	PathPrefix  string        `env:"PATH_PREFIX" usage:"prefix for emitted Node paths, e.g. 'platform' yields 'platform/github'"`
	Endpoints   string        `env:"MCP_ENDPOINTS" usage:"comma-separated MCP servers as name=url pairs"`
	BearerToken string        `env:"MCP_BEARER_TOKEN"`
	HTTPTimeout time.Duration `env:"MCP_HTTP_TIMEOUT" default:"10s"`
}

type Plugin struct {
	httpClient *http.Client
	logger     *log.Logger
	targets    []mcputil.Target
	nodePrefix string
}

func New(config config, logger *log.Logger) (*Plugin, error) {
	prefix := strings.Trim(strings.TrimSpace(config.PathPrefix), "/")
	if prefix == "" {
		return nil, fmt.Errorf("no node prefix configured: PATH_PREFIX is empty")
	}

	targets, err := parseEndpoints(config.Endpoints, config.BearerToken)
	if err != nil {
		return nil, fmt.Errorf("reading MCP_ENDPOINTS: %w", err)
	}

	return &Plugin{
		httpClient: &http.Client{Timeout: config.HTTPTimeout},
		logger:     logger,
		targets:    targets,
		nodePrefix: prefix,
	}, nil
}

func main() {
	app := pluginmain.New[config]()
	p, err := New(app.PluginConfig, app.Logger)
	if err != nil {
		log.Fatalf("failed to initialize plugin: %v", err)
	}
	app.Serve(p)
}

// parseEndpoints turns the "name=url,name=url" configuration into targets.
func parseEndpoints(raw string, bearerToken string) ([]mcputil.Target, error) {
	var targets []mcputil.Target
	seen := make(map[string]struct{})

	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		name, endpoint, ok := strings.Cut(entry, "=")
		if !ok {
			return nil, fmt.Errorf("invalid endpoint entry %q: must be in name=url format", entry)
		}
		name = strings.TrimSpace(name)
		endpoint = strings.TrimSpace(endpoint)
		if name == "" || endpoint == "" {
			return nil, fmt.Errorf("invalid endpoint entry %q: name and url must not be empty", entry)
		}
		if strings.Contains(name, "/") {
			return nil, fmt.Errorf("invalid endpoint name %q: must not contain '/'", name)
		}
		if _, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf("duplicate endpoint name %q", name)
		}
		seen[name] = struct{}{}

		targets = append(targets, mcputil.Target{
			Name:        name,
			Endpoint:    endpoint,
			BearerToken: bearerToken,
		})
	}

	if len(targets) == 0 {
		return nil, fmt.Errorf("no endpoints configured")
	}

	return targets, nil
}

func (p *Plugin) Collect(ctx context.Context) (pluginapi.CollectResponse, error) {
	nodes := make([]pluginapi.NodeClaim, 0, len(p.targets))
	relations := make([]pluginapi.RelationClaim, 0)

	for _, target := range p.targets {
		inventory, err := mcputil.Inspect(ctx, p.httpClient, target)
		if err != nil {
			p.logger.Printf("WARN: inspecting MCP server %q at %q: %v", target.Name, target.Endpoint, err)
			nodes = append(nodes, p.serverNode(target, inventory, err))
			continue
		}

		serverID := p.serverID(target.Name)
		nodes = append(nodes, p.serverNode(target, inventory, nil))

		for _, tool := range inventory.Tools {
			toolID := p.toolID(target.Name, tool.Name)
			nodes = append(nodes, pluginapi.NodeClaim{
				ID:         toolID,
				Properties: p.toolProperties(tool),
			})
			relations = append(relations, pluginapi.RelationClaim{
				Kind: pluginapi.RelationKindExposes,
				From: serverID,
				To:   toolID,
			})
		}
	}

	return pluginapi.CollectResponse{Nodes: nodes, Relations: relations}, nil
}

func (p *Plugin) serverID(name string) pluginapi.NodeID {
	return pluginapi.NodeID{Kind: pluginapi.NodeKindMCPServer, Path: p.nodePrefix + "/" + name}
}

func (p *Plugin) toolID(serverName, toolName string) pluginapi.NodeID {
	return pluginapi.NodeID{Kind: pluginapi.NodeKindMCPTool, Path: p.nodePrefix + "/" + serverName + "/" + toolName}
}

func (p *Plugin) serverNode(target mcputil.Target, inventory mcputil.Inventory, collectErr error) pluginapi.NodeClaim {
	properties := pluginapi.PropertyMap{
		propertyKeyEndpoint:  mcputil.RedactEndpoint(target.Endpoint),
		propertyKeyAuthType:  authType(target.BearerToken),
		propertyKeyReachable: strconv.FormatBool(collectErr == nil),
	}

	if collectErr != nil {
		properties[propertyKeyError] = truncate(collectErr.Error())
	}

	server := inventory.Server
	setNonEmpty(properties, propertyKeyProtocolVersion, server.ProtocolVersion)
	setNonEmpty(properties, propertyKeyServerName, server.Name)
	setNonEmpty(properties, propertyKeyServerTitle, server.Title)
	setNonEmpty(properties, propertyKeyServerVersion, server.Version)
	setNonEmpty(properties, propertyKeyWebsiteURL, server.WebsiteURL)
	setNonEmpty(properties, propertyKeyInstructions, truncate(server.Instructions))

	if collectErr == nil {
		properties[propertyKeyCapabilityTools] = strconv.FormatBool(server.Capabilities.Tools)
		properties[propertyKeyCapabilityResources] = strconv.FormatBool(server.Capabilities.Resources)
		properties[propertyKeyCapabilityPrompts] = strconv.FormatBool(server.Capabilities.Prompts)
		properties[propertyKeyCapabilityLogging] = strconv.FormatBool(server.Capabilities.Logging)
		properties[propertyKeyCapabilityCompletions] = strconv.FormatBool(server.Capabilities.Completions)
	}

	return pluginapi.NodeClaim{ID: p.serverID(target.Name), Properties: properties}
}

func (p *Plugin) toolProperties(tool mcputil.Tool) pluginapi.PropertyMap {
	properties := pluginapi.PropertyMap{}

	setNonEmpty(properties, propertyKeyTitle, tool.Title)
	setNonEmpty(properties, propertyKeyDescription, truncate(tool.Description))
	setNonEmpty(properties, propertyKeyInputSchema, string(tool.InputSchema))
	setNonEmpty(properties, propertyKeyOutputSchema, string(tool.OutputSchema))

	// Annotations are hints declared by the server
	if tool.Annotations != nil {
		setNonEmpty(properties, propertyKeyAnnotationTitle, tool.Annotations.Title)
		properties[propertyKeyReadOnlyHint] = strconv.FormatBool(tool.Annotations.ReadOnlyHint)
		properties[propertyKeyIdempotentHint] = strconv.FormatBool(tool.Annotations.IdempotentHint)
		if tool.Annotations.DestructiveHint != nil {
			properties[propertyKeyDestructiveHint] = strconv.FormatBool(*tool.Annotations.DestructiveHint)
		}
		if tool.Annotations.OpenWorldHint != nil {
			properties[propertyKeyOpenWorldHint] = strconv.FormatBool(*tool.Annotations.OpenWorldHint)
		}
	}

	return properties
}

func authType(bearerToken string) string {
	if strings.TrimSpace(bearerToken) == "" {
		return authTypeNone
	}
	return authTypeBearer
}

func setNonEmpty(properties pluginapi.PropertyMap, key, value string) {
	if value == "" {
		return
	}
	properties[key] = value
}

func truncate(value string) string {
	runes := []rune(value)
	if len(runes) <= maxTextLength {
		return value
	}

	return string(runes[:maxTextLength]) + "…"
}
