package httpapi

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/naira-project/naira/catalog/internal/catalog"
)

// NewMCPHandler exposes the catalog's node/relation/plugin operations as MCP
// tools, mirroring the REST routes registered in NewRouter.
func NewMCPHandler(service *catalog.Service, logger *log.Logger) http.Handler {
	server := newCatalogMCPServer(service, logger)
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
}

func newCatalogMCPServer(service *catalog.Service, logger *log.Logger) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "naira-catalog", Version: "v1.0.0"}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_nodes",
		Description: "List catalog nodes, optionally filtered by name, kind, or path.",
	}, listNodesToolHandler(service, logger))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_node",
		Description: "Get a single catalog node by kind and path.",
	}, getNodeToolHandler(service))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_relations",
		Description: "List catalog relations, optionally filtered by name, kind, fromNode, or toNode.",
	}, listRelationsToolHandler(service, logger))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_kinds",
		Description: "List the known catalog node and relation kinds.",
	}, listKindsToolHandler())

	mcp.AddTool(server, &mcp.Tool{
		Name:        "run_plugins",
		Description: "Run all registered catalog plugins to refresh nodes and relations.",
	}, runPluginsToolHandler(service))

	return server
}

type listNodesInput struct {
	PageSize  int32  `json:"pageSize,omitempty" jsonschema:"maximum number of nodes to return per page"`
	PageToken string `json:"pageToken,omitempty" jsonschema:"opaque token from a previous list_nodes call to fetch the next page"`
	Filter    string `json:"filter,omitempty" jsonschema:"equality filter in the form field=\"value\"; supported fields: name, kind, path"`
}

func listNodesToolHandler(service *catalog.Service, logger *log.Logger) mcp.ToolHandlerFor[listNodesInput, ListNodesResponse] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input listNodesInput) (*mcp.CallToolResult, ListNodesResponse, error) {
		options, err := listOptionsFromParams(pageSizeParam(input.PageSize), input.Filter, input.PageToken, nodeListOptionsSpec)
		if err != nil {
			return nil, ListNodesResponse{}, fmt.Errorf("getting list options: %w", err)
		}

		nodes := make([]Node, 0)
		for _, node := range service.ListNodes(ctx) {
			node := nodeFromCatalogNode(node)
			matches, err := matchNodeFilter(node, options.filter)
			if err != nil {
				return nil, ListNodesResponse{}, fmt.Errorf("matching node filter: %w", err)
			}
			if matches {
				nodes = append(nodes, node)
			}
		}

		sortNodes(nodes)

		page, nextPageToken, totalSize, err := paginate(nodes, options.pageSize, options.offset, "nodes", logger)
		if err != nil {
			return nil, ListNodesResponse{}, fmt.Errorf("paginating nodes: %w", err)
		}

		return nil, ListNodesResponse{Nodes: page, NextPageToken: nextPageToken, TotalSize: int32FromCount(totalSize, logger)}, nil
	}
}

type getNodeInput struct {
	Kind string `json:"kind" jsonschema:"the node kind, e.g. model, deployment, service"`
	Path string `json:"path" jsonschema:"the node path within its kind"`
}

func getNodeToolHandler(service *catalog.Service) mcp.ToolHandlerFor[getNodeInput, Node] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input getNodeInput) (*mcp.CallToolResult, Node, error) {
		node, err := service.GetNode(ctx, catalog.NodeID{Kind: input.Kind, Path: input.Path})
		if err != nil {
			return nil, Node{}, fmt.Errorf("getting node: %w", err)
		}

		return nil, nodeFromCatalogNode(node), nil
	}
}

type listRelationsInput struct {
	PageSize  int32  `json:"pageSize,omitempty" jsonschema:"maximum number of relations to return per page"`
	PageToken string `json:"pageToken,omitempty" jsonschema:"opaque token from a previous list_relations call to fetch the next page"`
	Filter    string `json:"filter,omitempty" jsonschema:"equality filter in the form field=\"value\"; supported fields: name, kind, fromNode, toNode"`
}

func listRelationsToolHandler(service *catalog.Service, logger *log.Logger) mcp.ToolHandlerFor[listRelationsInput, ListRelationsResponse] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input listRelationsInput) (*mcp.CallToolResult, ListRelationsResponse, error) {
		options, err := listOptionsFromParams(pageSizeParam(input.PageSize), input.Filter, input.PageToken, relationListOptionsSpec)
		if err != nil {
			return nil, ListRelationsResponse{}, fmt.Errorf("getting list options: %w", err)
		}

		relations := make([]Relation, 0)
		for _, relation := range service.ListRelations(ctx) {
			resource := relationFromCatalogRelation(relation)
			matches, err := matchRelationFilter(resource, options.filter)
			if err != nil {
				return nil, ListRelationsResponse{}, fmt.Errorf("matching relation filter: %w", err)
			}
			if matches {
				relations = append(relations, resource)
			}
		}

		sortRelations(relations)

		page, nextPageToken, totalSize, err := paginate(relations, options.pageSize, options.offset, "relations", logger)
		if err != nil {
			return nil, ListRelationsResponse{}, fmt.Errorf("paginating relations: %w", err)
		}

		return nil, ListRelationsResponse{Relations: page, NextPageToken: nextPageToken, TotalSize: int32FromCount(totalSize, logger)}, nil
	}
}

type listKindsInput struct{}

type ListKindsOutput struct {
	Kinds []string `json:"kinds"`
}

func listKindsToolHandler() mcp.ToolHandlerFor[listKindsInput, ListKindsOutput] {
	return func(_ context.Context, _ *mcp.CallToolRequest, _ listKindsInput) (*mcp.CallToolResult, ListKindsOutput, error) {
		return nil, ListKindsOutput{Kinds: catalogKinds}, nil
	}
}

type runPluginsInput struct{}

func runPluginsToolHandler(service *catalog.Service) mcp.ToolHandlerFor[runPluginsInput, RunPluginsResponse] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, _ runPluginsInput) (*mcp.CallToolResult, RunPluginsResponse, error) {
		result := service.RunAllPlugins(ctx)
		return nil, runPluginsResponseFromResult(result), nil
	}
}

func pageSizeParam(pageSize int32) string {
	if pageSize <= 0 {
		return ""
	}

	return strconv.FormatInt(int64(pageSize), 10)
}
