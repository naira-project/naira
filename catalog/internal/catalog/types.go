package catalog

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/robfig/cron/v3"
)

// PluginConfig is the immutable configuration for a registered plugin.
// An empty Schedule means that the plugin is manually triggered only.
type PluginConfig struct {
	Address  string
	Schedule string
}

type PluginConfigsByName map[string]PluginConfig

var ErrInvalidPluginConfig = errors.New("invalid plugin configuration")

func (c PluginConfigsByName) Validate() error {
	for name, config := range c {
		if name == "" {
			return fmt.Errorf("plugin name cannot be empty: %w", ErrInvalidPluginConfig)
		}
		if strings.ToLower(strings.TrimSpace(name)) != name {
			return fmt.Errorf("plugin name %q must be lowercased without leading/trailing whitespace: %w", name, ErrInvalidPluginConfig)
		}
		if config.Schedule != strings.TrimSpace(config.Schedule) {
			return fmt.Errorf("plugin %q schedule %q must not have leading or trailing whitespace: %w", name, config.Schedule, ErrInvalidPluginConfig)
		}
		if config.Address != strings.TrimSpace(config.Address) {
			return fmt.Errorf("plugin %q address %q must not have leading or trailing whitespace: %w", name, config.Address, ErrInvalidPluginConfig)
		}

		host, port, err := net.SplitHostPort(config.Address)
		if err != nil || host == "" || port == "" {
			if err == nil {
				err = errors.New("host or port is empty")
			}
			return fmt.Errorf("plugin %q has invalid address (expected host:port): %v: %w", name, err, ErrInvalidPluginConfig)
		}

		if config.Schedule != "" {
			if _, err := cron.ParseStandard(config.Schedule); err != nil {
				return fmt.Errorf("plugin %q has invalid schedule %v: %w", name, err, ErrInvalidPluginConfig)
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
