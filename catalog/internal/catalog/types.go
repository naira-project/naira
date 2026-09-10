package catalog

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/robfig/cron/v3"
)

// An empty Schedule means that the plugin is manually triggered only.
const ScheduleManual = ""

// PluginConfig is the immutable configuration for a registered plugin.
type PluginConfig struct {
	Address  string
	Schedule string
}

type PluginConfigsByName map[string]PluginConfig

var ErrInvalidPluginConfig = errors.New("invalid plugin configuration")

func (c PluginConfigsByName) Validate() error {
	for name, config := range c {
		switch {
		case name == "":
			return fmt.Errorf("plugin name cannot be empty: %w", ErrInvalidPluginConfig)
		case name != strings.ToLower(strings.TrimSpace(name)):
			return fmt.Errorf("plugin name %q must be lowercased without leading/trailing whitespace: %w", name, ErrInvalidPluginConfig)
		case config.Address == "":
			return fmt.Errorf("plugin %q has no address: %w", name, ErrInvalidPluginConfig)
		case config.Address != strings.TrimSpace(config.Address):
			return fmt.Errorf("plugin %q address %q must not have leading or trailing whitespace: %w", name, config.Address, ErrInvalidPluginConfig)
		case config.Schedule != strings.TrimSpace(config.Schedule):
			return fmt.Errorf("plugin %q schedule %q must not have leading or trailing whitespace: %w", name, config.Schedule, ErrInvalidPluginConfig)
		}

		host, port, err := net.SplitHostPort(config.Address)
		if err != nil || host == "" || port == "" {
			if err == nil {
				err = errors.New("host or port is empty")
			}
			return fmt.Errorf("plugin %q has invalid address (expected host:port): %v: %w", name, err, ErrInvalidPluginConfig)
		}

		if config.Schedule != ScheduleManual {
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
