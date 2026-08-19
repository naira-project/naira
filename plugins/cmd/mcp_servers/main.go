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
	"strings"
	"time"

	"github.com/naira-project/naira/plugins/internal/mcputil"
	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/naira-project/naira/plugins/pkg/pluginmain"
)

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
		}

		targetNodes, targetRelations := mcputil.Graph(p.nodePrefix, target, inventory, err, nil)
		nodes = append(nodes, targetNodes...)
		relations = append(relations, targetRelations...)
	}

	return pluginapi.CollectResponse{Nodes: nodes, Relations: relations}, nil
}
