package mcputil

import (
	"strconv"
	"strings"

	"github.com/naira-project/naira/plugins/pkg/pluginapi"
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

func ServerNodeID(prefix, serverName string) pluginapi.NodeID {
	return pluginapi.NodeID{Kind: pluginapi.NodeKindMCPServer, Path: prefix + "/" + serverName}
}

func ToolNodeID(prefix, serverName, toolName string) pluginapi.NodeID {
	return pluginapi.NodeID{Kind: pluginapi.NodeKindMCPTool, Path: prefix + "/" + serverName + "/" + toolName}
}

// Extracts catalog nodes and relations, so that
// An inspectErr still yields a server node, marked unreachable
func Graph(prefix string, target Target, inventory Inventory, inspectErr error, extra pluginapi.PropertyMap) ([]pluginapi.NodeClaim, []pluginapi.RelationClaim) {
	serverID := ServerNodeID(prefix, target.Name)

	nodes := []pluginapi.NodeClaim{{
		ID:         serverID,
		Properties: serverProperties(target, inventory, inspectErr, extra),
	}}
	relations := make([]pluginapi.RelationClaim, 0, len(inventory.Tools))

	if inspectErr != nil {
		return nodes, relations
	}

	for _, tool := range inventory.Tools {
		toolID := ToolNodeID(prefix, target.Name, tool.Name)
		nodes = append(nodes, pluginapi.NodeClaim{ID: toolID, Properties: toolProperties(tool)})
		relations = append(relations, pluginapi.RelationClaim{
			Kind: pluginapi.RelationKindExposes,
			From: serverID,
			To:   toolID,
		})
	}

	return nodes, relations
}

func serverProperties(target Target, inventory Inventory, inspectErr error, extra pluginapi.PropertyMap) pluginapi.PropertyMap {
	properties := pluginapi.PropertyMap{
		propertyKeyEndpoint:  RedactEndpoint(target.Endpoint),
		propertyKeyAuthType:  authType(target.BearerToken),
		propertyKeyReachable: strconv.FormatBool(inspectErr == nil),
	}

	if inspectErr != nil {
		properties[propertyKeyError] = truncate(inspectErr.Error())
	}

	server := inventory.Server
	setNonEmpty(properties, propertyKeyProtocolVersion, server.ProtocolVersion)
	setNonEmpty(properties, propertyKeyServerName, server.Name)
	setNonEmpty(properties, propertyKeyServerTitle, server.Title)
	setNonEmpty(properties, propertyKeyServerVersion, server.Version)
	setNonEmpty(properties, propertyKeyWebsiteURL, server.WebsiteURL)
	setNonEmpty(properties, propertyKeyInstructions, truncate(server.Instructions))

	if inspectErr == nil {
		properties[propertyKeyCapabilityTools] = strconv.FormatBool(server.Capabilities.Tools)
		properties[propertyKeyCapabilityResources] = strconv.FormatBool(server.Capabilities.Resources)
		properties[propertyKeyCapabilityPrompts] = strconv.FormatBool(server.Capabilities.Prompts)
		properties[propertyKeyCapabilityLogging] = strconv.FormatBool(server.Capabilities.Logging)
		properties[propertyKeyCapabilityCompletions] = strconv.FormatBool(server.Capabilities.Completions)
	}

	for key, value := range extra {
		setNonEmpty(properties, key, value)
	}

	return properties
}

func toolProperties(tool Tool) pluginapi.PropertyMap {
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

// truncate cuts on a rune boundary so clipping never produces invalid UTF-8.
func truncate(value string) string {
	runes := []rune(value)
	if len(runes) <= maxTextLength {
		return value
	}

	return string(runes[:maxTextLength]) + "…"
}
