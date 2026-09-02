package catalog

import "github.com/naira-project/naira/plugins/pkg/pluginapi"

// PluginDefinition is the immutable configuration for a registered plugin.
// An empty Schedule means that the plugin is manually triggered only.
type PluginDefinition struct {
	Address  string
	Schedule string
}

type PropertyMap = pluginapi.PropertyMap

type NodeID = pluginapi.NodeID

type CollectResponse = pluginapi.CollectResponse

type NodeClaim = pluginapi.NodeClaim

type RelationClaim = pluginapi.RelationClaim
