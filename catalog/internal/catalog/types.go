package catalog

import (
	"errors"
	"fmt"
	"strings"

	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/robfig/cron/v3"
)

// PluginDefinition is the immutable configuration for a registered plugin.
// An empty Schedule means that the plugin is manually triggered only.
type PluginDefinition struct {
	Address  string
	Schedule string
}

type PluginConfig map[string]PluginDefinition

var ErrInvalidPluginConfig = errors.New("invalid plugin configuration")

func (c PluginConfig) Validate() error {
	seen := make(map[string]string, len(c))
	for name, definition := range c {
		normalizedName := strings.ToLower(strings.TrimSpace(name))
		if normalizedName == "" {
			return fmt.Errorf("plugin name %q: %w", name, ErrInvalidPluginConfig)
		}
		if previous, exists := seen[normalizedName]; exists {
			return fmt.Errorf("plugin names %q and %q normalize to the same name: %w", previous, name, ErrInvalidPluginConfig)
		}
		seen[normalizedName] = name
		if strings.TrimSpace(definition.Address) == "" {
			return fmt.Errorf("plugin %q has no address: %w", name, ErrInvalidPluginConfig)
		}
		if definition.Schedule != "" {
			if _, err := cron.ParseStandard(definition.Schedule); err != nil {
				return fmt.Errorf("plugin %q has invalid schedule %q: %w", name, definition.Schedule, ErrInvalidPluginConfig)
			}
		}
	}
	return nil
}

type PropertyMap = pluginapi.PropertyMap

type NodeID = pluginapi.NodeID

type CollectResponse = pluginapi.CollectResponse

type NodeClaim = pluginapi.NodeClaim

type RelationClaim = pluginapi.RelationClaim
