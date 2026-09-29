package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/naira-project/naira/plugins/internal/mcputil"
	"github.com/naira-project/naira/plugins/pkg/pluginapi"
)

// LiteLLM specific info that an MCP server does not self report.
const (
	propertyKeyDescription  = "description"
	propertyKeyAlias        = "litellm_alias"
	propertyKeyServerID     = "litellm_server_id"
	propertyKeyUpstreamAuth = "litellm_upstream_auth_type"
	propertyKeyAccessGroups = "litellm_access_groups"
	propertyKeyTeams        = "litellm_teams"
	propertyKeyStatus       = "litellm_status"
)

type mcpServer struct {
	ServerID     string   `json:"server_id"`
	ServerName   string   `json:"server_name"`
	Alias        string   `json:"alias"`
	Description  string   `json:"description"`
	AuthType     string   `json:"auth_type"`
	Status       string   `json:"status"`
	Teams        []string `json:"teams"`
	AccessGroups []string `json:"mcp_access_groups"`
}

func (p *Plugin) collectMCPServers(ctx context.Context) ([]pluginapi.NodeClaim, []pluginapi.RelationClaim, error) {
	servers, err := p.fetchMCPServers(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("listing LiteLLM MCP servers: %w", err)
	}

	var (
		nodes     []pluginapi.NodeClaim
		relations []pluginapi.RelationClaim
	)

	for _, server := range servers {
		name := strings.TrimSpace(server.ServerName)
		if name == "" || strings.Contains(name, "/") {
			if p.logger != nil {
				p.logger.Printf("WARN: skipping LiteLLM MCP server with a not valid name %q", server.ServerName)
			}
			continue
		}

		target := mcputil.Target{
			Name:        name,
			Endpoint:    strings.TrimRight(p.config.BaseURL, "/") + "/" + name + "/mcp",
			BearerToken: p.config.APIKey,
		}

		inventory, err := mcputil.Inspect(ctx, p.httpClient, target)
		if err != nil && p.logger != nil {
			p.logger.Printf("WARN: inspecting LiteLLM MCP server %q: %v", name, err)
		}

		serverNodes, serverRelations := mcputil.Graph(p.config.PathPrefix, target, inventory, err, server.properties())
		nodes = append(nodes, serverNodes...)
		relations = append(relations, serverRelations...)
	}

	return nodes, relations, nil
}

func (s mcpServer) properties() pluginapi.PropertyMap {
	properties := pluginapi.PropertyMap{}
	for key, value := range map[string]string{
		propertyKeyDescription:  s.Description,
		propertyKeyAlias:        s.Alias,
		propertyKeyServerID:     s.ServerID,
		propertyKeyUpstreamAuth: s.AuthType,
		propertyKeyStatus:       s.Status,
		propertyKeyTeams:        strings.Join(s.Teams, ","),
		propertyKeyAccessGroups: strings.Join(s.AccessGroups, ","),
	} {
		if value != "" {
			properties[key] = value
		}
	}

	return properties
}

func (p *Plugin) fetchMCPServers(ctx context.Context) ([]mcpServer, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.config.BaseURL+"/v1/mcp/server", nil)
	if err != nil {
		return nil, fmt.Errorf("building LiteLLM MCP server request: %w", err)
	}
	p.addAuthorization(req)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling LiteLLM MCP server endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("litellm /v1/mcp/server returned %s", resp.Status)
	}

	var servers []mcpServer
	if err := json.NewDecoder(resp.Body).Decode(&servers); err != nil {
		return nil, fmt.Errorf("decoding LiteLLM MCP server response: %w", err)
	}

	return servers, nil
}
