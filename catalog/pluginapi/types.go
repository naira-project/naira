package pluginapi

import (
	"context"
)

type Plugin interface {
	Name() string
	Collect(context.Context) (IngestionRequest, error)
}

type PropertyMap map[string]string

// NodeID identifies a node within the catalog graph.
// Kind must not contain '/' because it is used as a single URL path segment.
// Path is opaque and may contain '/'.
type NodeID struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
}

type IngestionRequest struct {
	Nodes     []NodeClaim     `json:"nodes"`
	Relations []RelationClaim `json:"relations"`
}

type NodeClaim struct {
	ID         NodeID      `json:"id"`
	Properties PropertyMap `json:"properties,omitempty"`
}

type RelationClaim struct {
	Kind       string      `json:"kind"` // Kind must not contain '/'
	From       NodeID      `json:"from"`
	To         NodeID      `json:"to"`
	Properties PropertyMap `json:"properties,omitempty"`
}
