package httpapi

import (
	"fmt"
	"log"
	"net/http"
	"net/url"

	"github.com/naira-project/naira/catalog/internal/catalog"
	"github.com/naira-project/naira/plugins/pkg/pluginapi"
)

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

var relationListOptionsSpec = listOptionsSpec{
	scope:         "relations",
	allowedFields: map[string]bool{"name": true, "kind": true, "fromNode": true, "toNode": true},
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

func sortRelations(relations []Relation) {
	sortResources(relations, func(relation Relation) string {
		return relation.Name
	})
}

func relationName(kind string, from pluginapi.NodeID, to pluginapi.NodeID) string {
	return fmt.Sprintf("relations/%s/%s|%s", kind, url.PathEscape(nodeName(from)), url.PathEscape(nodeName(to)))
}

// GET /v1/relations lists catalog relations.
// Supported query params:
// - pageSize
// - pageToken
// - filter: only field="value" equality filters
// Supported relation filter fields: name, kind, fromNode, toNode.
func newListRelationsHandler(service *catalog.Service, logger *log.Logger) http.HandlerFunc {
	return handleWithListOptions(relationListOptionsSpec, func(w http.ResponseWriter, r *http.Request, options listOptions) error {
		relations := make([]Relation, 0)
		for _, relation := range service.ListRelations(r.Context()) {
			resource := relationFromCatalogRelation(relation)
			matches, err := matchRelationFilter(resource, options.filter)
			if err != nil {
				return fmt.Errorf("matching relation filter: %w", err)
			}
			if matches {
				relations = append(relations, resource)
			}
		}

		sortRelations(relations)

		page, nextPageToken, totalSize, err := paginate(relations, options.pageSize, options.offset, "relations", logger)
		if err != nil {
			return fmt.Errorf("paginating relations: %w", err)
		}

		writeJSON(w, http.StatusOK, ListRelationsResponse{Relations: page, NextPageToken: nextPageToken, TotalSize: int32FromCount(totalSize, logger)})
		return nil
	})
}
