package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/naira-project/naira/catalog/internal/catalog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClusterCredentialEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plugins.yaml")
	require.NoError(t, os.WriteFile(path, []byte("plugins: {}"), 0o600))
	t.Setenv("PLUGIN_CONFIG_FILE", path)
	for _, tt := range []struct {
		name  string
		value string
		want  map[string][]string
	}{
		{name: "unset"},
		{name: "empty map", value: `{}`, want: map[string][]string{}},
		{
			name:  "workspace allowlists",
			value: `{"workspace-a":["idp-system/first#kubeconfig","idp-system/shared#config"],"workspace-b":["idp-system/second#config","idp-system/shared#config"]}`,
			want: map[string][]string{
				"workspace-a": {"idp-system/first#kubeconfig", "idp-system/shared#config"},
				"workspace-b": {"idp-system/second#config", "idp-system/shared#config"},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CLUSTER_CREDENTIAL_SECRETS", tt.value)
			config, err := loadConfig()
			require.NoError(t, err)
			assert.Equal(t, tt.want, config.ClusterCredentialSecrets)
		})
	}
	for _, value := range []string{
		`idp-system/first#kubeconfig,idp-system/second#config`,
		`["idp-system/first#kubeconfig"]`,
		`{"workspace-a":"idp-system/first#kubeconfig"}`,
		`{"workspace-a":[`,
	} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("CLUSTER_CREDENTIAL_SECRETS", value)
			_, err := loadConfig()
			require.ErrorContains(t, err, "parse workspace credential allowlists")
		})
	}
}

func TestLoadPluginConfig(t *testing.T) {
	tests := []struct {
		name             string
		config           string
		skipFileCreation bool
		wantErrText      string
		wantPlugins      catalog.PluginConfigsByName
	}{
		{
			name:             "returns an error when the plugin configuration file is missing",
			skipFileCreation: true,
			wantErrText:      "read plugin configuration file",
		},
		{
			name:        "returns an error when the plugin configuration is invalid YAML",
			config:      "plugins: [",
			wantErrText: "parse plugin configuration file",
		},
		{
			name: "check if validation is triggered",
			config: `plugins:
  mlflow:
    schedule: "*/5 * * * *"
`,
			wantErrText: `plugin "mlflow" has no address`,
		},
		{
			name: "successfully loads config with plugins",
			config: `plugins:
  mlflow:
    address: "localhost:50051"
    schedule: "*/5 * * * *"
`,
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
			if !tt.skipFileCreation {
				require.NoError(t, os.WriteFile(configPath, []byte(tt.config), 0o600))
			}

			plugins, err := loadAndValidatePluginConfig(configPath)

			if tt.wantErrText != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.wantErrText)
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
		err     string
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
		},
		{
			name: "fails when plugin name is empty",
			plugins: catalog.PluginConfigsByName{
				"": {
					Address: "localhost:50051",
				},
			},
			err: "plugin name cannot be empty",
		},
		{
			name: "fails when plugin name contains uppercase characters",
			plugins: catalog.PluginConfigsByName{
				"MLFlow": {
					Address: "localhost:50051",
				},
			},
			err: "must be lowercased without leading/trailing whitespace",
		},
		{
			name: "fails when plugin name has leading or trailing spaces",
			plugins: catalog.PluginConfigsByName{
				" mlflow ": {
					Address: "localhost:50051",
				},
			},
			err: "must be lowercased without leading/trailing whitespace",
		},
		{
			name: "fails when schedule has leading or trailing whitespace",
			plugins: catalog.PluginConfigsByName{
				"mlflow": {
					Address:  "localhost:50051",
					Schedule: " */5 * * * * ",
				},
			},
			err: "must not have leading or trailing whitespace",
		},
		{
			name: "fails when address is missing port",
			plugins: catalog.PluginConfigsByName{
				"mlflow": {
					Address: "localhost",
				},
			},
			err: "has invalid address (expected host:port)",
		},
		{
			name: "fails when address includes http scheme",
			plugins: catalog.PluginConfigsByName{
				"mlflow": {
					Address: "http://localhost:50051",
				},
			},
			err: "has invalid address (expected host:port)",
		},
		{
			name: "fails when address is empty",
			plugins: catalog.PluginConfigsByName{
				"mlflow": {
					Address: "",
				},
			},
			err: "has no address",
		},
		{
			name: "fails when schedule cron format is invalid",
			plugins: catalog.PluginConfigsByName{
				"mlflow": {
					Address:  "localhost:50051",
					Schedule: "@Daily",
				},
			},
			err: "has invalid schedule",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.plugins.Validate()

			if tt.err != "" {
				assert.ErrorContains(t, err, tt.err)
				assert.ErrorIs(t, err, catalog.ErrInvalidPluginConfig)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
