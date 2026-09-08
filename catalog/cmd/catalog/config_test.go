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
		wantPlugins catalog.PluginConfigsByName
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
			wantPlugins: catalog.PluginConfigsByName{
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

func TestPluginConfigsByName_Validate(t *testing.T) {
	tests := []struct {
		name    string
		plugins catalog.PluginConfigsByName
		wantErr bool
	}{
		{
			name: "valid configuration passes",
			plugins: catalog.PluginConfigsByName{
				"mlflow": {
					Address:  "localhost:50051",
					Schedule: "*/5 * * * *",
				},
				"github": {
					Address:  "127.0.0.1:8080",
					Schedule: "",
				},
				"external-plugin": {
					Address:  "plugin.example.com:443",
					Schedule: "@daily",
				},
			},
			wantErr: false,
		},
		{
			name: "fails when plugin name is empty",
			plugins: catalog.PluginConfigsByName{
				"": {
					Address: "localhost:50051",
				},
			},
			wantErr: true,
		},
		{
			name: "fails when plugin name contains uppercase characters",
			plugins: catalog.PluginConfigsByName{
				"MLFlow": {
					Address: "localhost:50051",
				},
			},
			wantErr: true,
		},
		{
			name: "fails when plugin name has leading or trailing spaces",
			plugins: catalog.PluginConfigsByName{
				" mlflow ": {
					Address: "localhost:50051",
				},
			},
			wantErr: true,
		},
		{
			name: "fails when schedule has leading or trailing whitespace",
			plugins: catalog.PluginConfigsByName{
				"mlflow": {
					Address:  "localhost:50051",
					Schedule: " */5 * * * * ",
				},
			},
			wantErr: true,
		},
		{
			name: "fails when address is missing port",
			plugins: catalog.PluginConfigsByName{
				"mlflow": {
					Address: "localhost",
				},
			},
			wantErr: true,
		},
		{
			name: "fails when address includes http scheme",
			plugins: catalog.PluginConfigsByName{
				"mlflow": {
					Address: "http://localhost:50051",
				},
			},
			wantErr: true,
		},
		{
			name: "fails when address is empty",
			plugins: catalog.PluginConfigsByName{
				"mlflow": {
					Address: "",
				},
			},
			wantErr: true,
		},
		{
			name: "fails when schedule cron format is invalid",
			plugins: catalog.PluginConfigsByName{
				"mlflow": {
					Address:  "localhost:50051",
					Schedule: "@Daily",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.plugins.Validate()

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, catalog.ErrInvalidPluginConfig)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
