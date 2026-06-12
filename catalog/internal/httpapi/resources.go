package httpapi

import (
	"fmt"
	"log"
	"maps"
	"math"
	"net/url"
	"slices"
	"strings"

	"github.com/naira-project/naira/catalog/internal/catalog"
	"github.com/naira-project/naira/catalog/pluginapi"
)

type PluginClaim struct {
	Plugin string            `json:"plugin"`
	Props  map[string]string `json:"props"`
}

type Node struct {
	Name         string        `json:"name"`
	Kind         string        `json:"kind"`
	Path         string        `json:"path"`
	PluginClaims []PluginClaim `json:"pluginClaimss"`
}

type ListNodesResponse struct {
	Nodes         []Node `json:"nodes"`
	NextPageToken string `json:"nextPageToken,omitempty"`
	TotalSize     int32  `json:"totalSize"`
}

type Relation struct {
	Name         string        `json:"name"`
	Kind         string        `json:"kind"`
	FromNode     string        `json:"fromNode"`
	ToNode       string        `json:"toNode"`
	PluginClaims []PluginClaim `json:"pluginClaims"`
}

type ListRelationsResponse struct {
	Relations     []Relation `json:"relations"`
	NextPageToken string     `json:"nextPageToken,omitempty"`
	TotalSize     int32      `json:"totalSize"`
}

type RunPluginResult struct {
	Plugin string `json:"plugin"`
	Error  string `json:"error"`
}

type RunPluginsResponse struct {
	Results []RunPluginResult `json:"results"`
}

func nodeFromCatalogNode(node catalog.Node) Node {
	return Node{
		Name:         nodeName(node.ID),
		Kind:         node.ID.Kind,
		Path:         node.ID.Path,
		PluginClaims: toSortedSlice(node.PluginClaims),
	}
}

func relationFromCatalogRelation(relation catalog.Relation) Relation {
	return Relation{
		Name:         relationName(relation.Kind, relation.From, relation.To),
		Kind:         relation.Kind,
		FromNode:     nodeName(relation.From),
		ToNode:       nodeName(relation.To),
		PluginClaims: toSortedSlice(relation.PluginClaims),
	}
}

func runPluginsResponseFromResult(result catalog.RunPluginsResult) RunPluginsResponse {
	response := RunPluginsResponse{
		Results: make([]RunPluginResult, 0, len(result.Results)),
	}

	for _, item := range result.Results {
		response.Results = append(response.Results, RunPluginResult{
			Plugin: item.Plugin,
			Error:  item.Error,
		})
	}

	return response
}

func matchNodeFilter(node Node, filter *equalityFilter) (bool, error) {
	matches, err := filter.matchesResource(map[string]string{
		"name": node.Name,
		"kind": node.Kind,
		"path": node.Path,
	}, "node")
	if err != nil {
		return false, fmt.Errorf("matching resource filter: %w", err)
	}

	return matches, nil
}

var nodeListOptionsSpec = listOptionsSpec{
	scope:         "nodes",
	allowedFields: map[string]bool{"name": true, "kind": true, "path": true},
}

func matchRelationFilter(relation Relation, filter *equalityFilter) (bool, error) {
	matches, err := filter.matchesResource(map[string]string{
		"name":     relation.Name,
		"kind":     relation.Kind,
		"fromNode": relation.FromNode,
		"toNode":   relation.ToNode,
	}, "relation")
	if err != nil {
		return false, fmt.Errorf("matching resource filter: %w", err)
	}

	return matches, nil
}

var relationListOptionsSpec = listOptionsSpec{
	scope:         "relations",
	allowedFields: map[string]bool{"name": true, "kind": true, "fromNode": true, "toNode": true},
}

func sortNodes(nodes []Node) {
	sortResources(nodes, func(node Node) string {
		return node.Name
	})
}

func sortRelations(relations []Relation) {
	sortResources(relations, func(relation Relation) string {
		return relation.Name
	})
}

func nodeName(id pluginapi.NodeID) string {
	return fmt.Sprintf("nodes/%s/%s", id.Kind, id.Path)
}

func relationName(kind string, from pluginapi.NodeID, to pluginapi.NodeID) string {
	return fmt.Sprintf("relations/%s/%s|%s", kind, url.PathEscape(nodeName(from)), url.PathEscape(nodeName(to)))
}

func toSortedSlice(claims map[string]catalog.PluginClaim) []PluginClaim {
	result := make([]PluginClaim, 0, len(claims))
	for pluginName, claim := range claims {
		copiedProps := make(map[string]string, len(claim.Properties))
		maps.Copy(copiedProps, claim.Properties)
		result = append(result, PluginClaim{
			Plugin: pluginName,
			Props:  copiedProps,
		})
	}

	slices.SortFunc(result, func(a, b PluginClaim) int {
		return strings.Compare(a.Plugin, b.Plugin)
	})

	return result
}

func int32FromCount(value int, logger *log.Logger) int32 {
	if value > math.MaxInt32 {
		logger.Printf("count %d exceeds int32 range", value)
		return math.MaxInt32
	}

	return int32(value)
}
