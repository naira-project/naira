package catalog

import "github.com/naira-project/naira/catalog/pluginapi"

type Plugin = pluginapi.Plugin

type PropertyMap = pluginapi.PropertyMap

type NodeID = pluginapi.NodeID

type IngestionRequest = pluginapi.IngestionRequest

type NodeClaim = pluginapi.NodeClaim

type RelationClaim = pluginapi.RelationClaim

type RunPluginResult struct {
	Plugin string
	Error  string
}

type RunPluginsResult struct {
	Results []RunPluginResult
}
