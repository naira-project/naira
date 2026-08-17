package httpapi

import (
	"fmt"
	"log"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"

	"github.com/naira-project/naira/catalog/internal/catalog"
	"github.com/naira-project/naira/plugins/pkg/pluginapi"
)

type Node struct {
	Name         string        `json:"name"`
	Kind         string        `json:"kind"`
	Path         string        `json:"path"`
	PluginClaims []PluginClaim `json:"pluginClaims"`
}

type ListNodesResponse struct {
	Nodes         []Node `json:"nodes"`
	NextPageToken string `json:"nextPageToken,omitempty"`
	TotalSize     int32  `json:"totalSize"`
}

var nodeListOptionsSpec = listOptionsSpec{
	scope:         "nodes",
	allowedFields: map[string]bool{"name": true, "kind": true, "path": true},
}

func nodeFromCatalogNode(node catalog.Node) Node {
	return Node{
		Name:         nodeName(node.ID),
		Kind:         node.ID.Kind,
		Path:         node.ID.Path,
		PluginClaims: toSortedSlice(node.PluginClaims),
	}
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

func sortNodes(nodes []Node) {
	sortResources(nodes, func(node Node) string {
		return node.Name
	})
}

func nodeName(id pluginapi.NodeID) string {
	return fmt.Sprintf("nodes/%s/%s", id.Kind, id.Path)
}

// GET /v1/nodes lists catalog nodes.
// Supported query params:
// - pageSize
// - pageToken
// - filter: only field="value" equality filters
// Supported node filter fields: name, kind, path.
func newListNodesHandler(service *catalog.Service, logger *log.Logger) http.HandlerFunc {
	return handleWithListOptions(nodeListOptionsSpec, func(w http.ResponseWriter, r *http.Request, options listOptions) error {
		nodes := make([]Node, 0)
		for _, node := range service.ListNodes(r.Context()) {
			node := nodeFromCatalogNode(node)
			matches, err := matchNodeFilter(node, options.filter)
			if err != nil {
				return fmt.Errorf("matching node filter: %w", err)
			}
			if matches {
				nodes = append(nodes, node)
			}
		}

		sortNodes(nodes)

		page, nextPageToken, totalSize, err := paginate(nodes, options.pageSize, options.offset, "nodes", logger)
		if err != nil {
			return fmt.Errorf("paginating nodes: %w", err)
		}

		writeJSON(w, http.StatusOK, ListNodesResponse{Nodes: page, NextPageToken: nextPageToken, TotalSize: int32FromCount(totalSize, logger)})
		return nil
	})
}

// GET /v1/nodes/{kind}/* returns a single catalog node.
func newGetNodeHandler(service *catalog.Service) http.HandlerFunc {
	return handle(func(w http.ResponseWriter, r *http.Request) error {
		path, err := url.PathUnescape(chi.URLParam(r, "*"))
		if err != nil {
			return fmt.Errorf("unescaping node path: %w", err)
		}

		node, err := service.GetNode(r.Context(), catalog.NodeID{
			Kind: chi.URLParam(r, "kind"),
			Path: path,
		})
		if err != nil {
			return fmt.Errorf("getting node: %w", err)
		}

		writeJSON(w, http.StatusOK, nodeFromCatalogNode(node))
		return nil
	})
}
