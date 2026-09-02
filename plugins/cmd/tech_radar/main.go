// tech_radar plugin that renders an admin-managed technology radar
// configuration file into the catalog. The YAML file is the sole source of
// truth: it defines the radar metadata, quadrant and ring taxonomies, and the
// entries with their adoption ring, movement marker, owner, and rationale.
//
// The file is re-read on every collect, so ConfigMap updates take effect on
// the next sync without a redeployment. An invalid file fails the collect
// with errors naming the offending line and field, which keeps the previous
// radar snapshot in place (the catalog never applies a failed run).
//
// # Environment Variables
//
//   - TECH_RADAR_CONFIG_PATH (optional) - path to the radar YAML file;
//     defaults to /etc/naira/techradar/radar.yaml.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/naira-project/naira/plugins/pkg/pluginmain"
)

type config struct {
	ConfigPath string `env:"TECH_RADAR_CONFIG_PATH" default:"/etc/naira/techradar/radar.yaml" usage:"path to the tech radar YAML file, re-read on every collect"`
}

type Plugin struct {
	configPath string
	logger     *log.Logger
}

func New(config config, logger *log.Logger) (*Plugin, error) {
	if config.ConfigPath == "" {
		return nil, fmt.Errorf("no config file configured: TECH_RADAR_CONFIG_PATH is empty")
	}

	return &Plugin{
		configPath: config.ConfigPath,
		logger:     logger,
	}, nil
}

func main() {
	app := pluginmain.New[config]()
	p, err := New(app.PluginConfig, app.Logger)
	if err != nil {
		log.Fatalf("failed to initialize plugin: %v", err)
	}
	app.Serve(p)
}

func (p *Plugin) Collect(_ context.Context) (pluginapi.CollectResponse, error) {
	data, err := os.ReadFile(p.configPath)
	if err != nil {
		return pluginapi.CollectResponse{}, fmt.Errorf("reading tech radar config %q: %w", p.configPath, err)
	}

	cfg, err := parseRadarConfig(data)
	if err != nil {
		return pluginapi.CollectResponse{}, fmt.Errorf("loading tech radar config %q: %w", p.configPath, err)
	}

	return p.buildResponse(cfg), nil
}
