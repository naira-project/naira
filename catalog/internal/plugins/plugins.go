package plugins

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/hashicorp/go-plugin"
	"github.com/naira-project/naira/catalog/pluginapi"
)

func Register(config Config, httpClient *http.Client, logger *log.Logger) ([]pluginapi.Plugin, func(), error) {
	var registered []pluginapi.Plugin
	var cleanups []func()

	// if config.MLflow.Enabled {
	// 	registered = appendIfNotNil(registered, mlflow.New(httpClient, config.MLflow))
	// }

	// if config.LiteLLM.Enabled {
	// 	registered = appendIfNotNil(registered, litellm.New(httpClient, logger, config.LiteLLM))
	// }

	if config.PluginsDir != "" {
		entries, err := os.ReadDir(config.PluginsDir)
		if err != nil {
			if !os.IsNotExist(err) {
				for _, cleanup := range cleanups {
					cleanup()
				}
				return nil, nil, fmt.Errorf("reading plugins directory %q: %w", config.PluginsDir, err)
			}
		} else {
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}

				info, err := entry.Info()
				if err != nil {
					continue
				}

				// Check if the file is executable
				if info.Mode()&0111 == 0 {
					continue
				}

				path := filepath.Join(config.PluginsDir, entry.Name())
				extPlugin, cleanup, err := LoadExternalPlugin(path)
				if err != nil {
					for _, c := range cleanups {
						c()
					}
					return nil, nil, fmt.Errorf("loading external plugin %q: %w", path, err)
				}

				if logger != nil {
					logger.Printf("successfully loaded external plugin %q from %q", extPlugin.Name(), path)
				}
				registered = append(registered, extPlugin)
				cleanups = append(cleanups, cleanup)
			}
		}
	}

	cleanupAll := func() {
		for _, cleanup := range cleanups {
			cleanup()
		}
	}

	return registered, cleanupAll, nil
}

func LoadExternalPlugin(pathToBinary string) (pluginapi.Plugin, func(), error) {
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: pluginapi.HandshakeConfig,
		Plugins: map[string]plugin.Plugin{
			"catalog-plugin": &pluginapi.HashiPlugin{},
		},
		Cmd:              exec.Command(pathToBinary),
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
	})

	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return nil, nil, fmt.Errorf("starting plugin client for binary %q: %w", pathToBinary, err)
	}

	raw, err := rpcClient.Dispense("catalog-plugin")
	if err != nil {
		client.Kill()
		return nil, nil, fmt.Errorf("dispensing plugin for binary %q: %w", pathToBinary, err)
	}

	p, ok := raw.(pluginapi.Plugin)
	if !ok {
		client.Kill()
		return nil, nil, fmt.Errorf("dispensed plugin is not a pluginapi.Plugin")
	}

	return p, func() { client.Kill() }, nil
}

func appendIfNotNil(plugins []pluginapi.Plugin, plugin pluginapi.Plugin) []pluginapi.Plugin {
	if plugin != nil {
		return append(plugins, plugin)
	}
	return plugins
}
