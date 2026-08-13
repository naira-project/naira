package httpapi

import (
	"maps"
	"slices"
	"strings"

	"github.com/naira-project/naira/catalog/internal/catalog"
)

// PluginClaim is shared by the Node and Relation resources: both are
// composed of per-plugin claims about their properties.
type PluginClaim struct {
	Plugin string            `json:"plugin"`
	Props  map[string]string `json:"props"`
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
