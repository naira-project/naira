package catalog

import pluginapi "github.com/naira-project/naira/plugins/pkg/api"

type Plugin = pluginapi.Plugin

type PropertyMap = pluginapi.PropertyMap

type NodeID = pluginapi.NodeID

type CollectResponse = pluginapi.CollectResponse

type NodeClaim = pluginapi.NodeClaim

type RelationClaim = pluginapi.RelationClaim

type RunPluginResult struct {
	Plugin string
	Error  string
}

type RunPluginsResult struct {
	Results []RunPluginResult
}
