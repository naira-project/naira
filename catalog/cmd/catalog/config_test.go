package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/naira-project/naira/catalog/internal/catalog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadPluginConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      string
		createFile  bool
		wantErrText string
		wantPlugins map[string]catalog.PluginDefinition
	}{
		{
			name:        "returns an error when the plugin configuration file is missing",
			wantErrText: "read plugin configuration file",
		},
		{
			name:        "returns an error when the plugin configuration is invalid YAML",
			config:      "plugins: [",
			createFile:  true,
			wantErrText: "parse plugin configuration file",
		},
		{
			name: "returns an error when a plugin has no address",
			config: `plugins:
  mlflow:
    schedule: "*/5 * * * *"
`,
			createFile:  true,
			wantErrText: `plugin "mlflow" has no address`,
		},
		{
			name: "successfully loads config with plugins",
			config: `plugins:
  mlflow:
    address: "localhost:50051"
    schedule: "*/5 * * * *"
`,
			createFile: true,
			wantPlugins: map[string]catalog.PluginDefinition{
				"mlflow": {
					Address:  "localhost:50051",
					Schedule: "*/5 * * * *",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "plugins.yaml")
			if tt.createFile {
				require.NoError(t, os.WriteFile(configPath, []byte(tt.config), 0o600))
			}

			plugins, err := loadPluginConfig(configPath)

			if tt.wantErrText != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrText)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantPlugins, plugins)
			}
		})
	}
}
