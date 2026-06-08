package pluginmanager

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const mockPluginSource = `
package main

import (
	"context"

	"github.com/hashicorp/go-plugin"
	"github.com/naira-project/naira/catalog/pluginapi"
)

type mockPlugin struct{}

func (mockPlugin) Name() string {
	return "mock-external-plugin"
}

func (mockPlugin) Collect(ctx context.Context) (pluginapi.IngestionRequest, error) {
	return pluginapi.IngestionRequest{
		Nodes: []pluginapi.NodeClaim{
			{
				ID: pluginapi.NodeID{Kind: "model", Path: "mock/model"},
				Properties: pluginapi.PropertyMap{"test": "val"},
			},
		},
		Relations: []pluginapi.RelationClaim{
			{
				Kind: "uses_model",
				From: pluginapi.NodeID{Kind: "application", Path: "mock/app"},
				To:   pluginapi.NodeID{Kind: "model", Path: "mock/model"},
			},
		},
	}, nil
}

func main() {
	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: pluginapi.HandshakeConfig,
		Plugins: map[string]plugin.Plugin{
			"catalog-plugin": &pluginapi.HashiPlugin{Impl: mockPlugin{}},
		},
		GRPCServer: plugin.DefaultGRPCServer,
	})
}
`

func TestLoadExternalPlugin(t *testing.T) {
	tmpDir := t.TempDir()

	// Write mock plugin source
	srcPath := filepath.Join(tmpDir, "main.go")
	err := os.WriteFile(srcPath, []byte(mockPluginSource), 0644)
	require.NoError(t, err)

	// Compile the mock plugin binary
	binPath := filepath.Join(tmpDir, "mock-plugin")
	cmd := exec.Command("go", "build", "-o", binPath, srcPath)
	// Inherit some env variables like PATH/GOCACHE to make sure compilation works
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "failed to compile mock plugin binary: %s", string(output))

	// Load the external plugin
	extPlugin, cleanup, err := LoadExternalPlugin(binPath)
	require.NoError(t, err)
	defer cleanup()

	// Assert plugin properties and methods
	assert.Equal(t, "mock-external-plugin", extPlugin.Name())

	req, err := extPlugin.Collect(t.Context())
	require.NoError(t, err)

	require.Len(t, req.Nodes, 1)
	assert.Equal(t, "model", req.Nodes[0].ID.Kind)
	assert.Equal(t, "mock/model", req.Nodes[0].ID.Path)
	assert.Equal(t, "val", req.Nodes[0].Properties["test"])

	require.Len(t, req.Relations, 1)
	assert.Equal(t, "uses_model", req.Relations[0].Kind)
	assert.Equal(t, "mock/app", req.Relations[0].From.Path)
	assert.Equal(t, "mock/model", req.Relations[0].To.Path)
}

func TestRegisterWithExternalPlugins(t *testing.T) {
	tmpDir := t.TempDir()
	pluginsDir := filepath.Join(tmpDir, "bin")
	err := os.Mkdir(pluginsDir, 0755)
	require.NoError(t, err)

	// Write mock plugin source
	srcPath := filepath.Join(tmpDir, "main.go")
	err = os.WriteFile(srcPath, []byte(mockPluginSource), 0644)
	require.NoError(t, err)

	// Compile the mock plugin binary directly into pluginsDir
	binPath := filepath.Join(pluginsDir, "mock-plugin-binary")
	cmd := exec.Command("go", "build", "-o", binPath, srcPath)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "failed to compile mock plugin binary: %s", string(output))

	// Make sure the binary has executable permissions
	err = os.Chmod(binPath, 0755)
	require.NoError(t, err)

	registered, cleanup, err := Register(pluginsDir, nil, nil)
	require.NoError(t, err)
	defer cleanup()

	require.Len(t, registered, 1)
	assert.Equal(t, "mock-external-plugin", registered[0].Name())
}
