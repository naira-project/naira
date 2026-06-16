package pluginmanager

import (
	"context"
	"net"
	"testing"

	"github.com/naira-project/naira/catalog/pluginapi"
	"github.com/naira-project/naira/catalog/pluginapi/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type mockPlugin struct{}

func (mockPlugin) Collect(ctx context.Context) (pluginapi.IngestionRequest, error) {
	return pluginapi.IngestionRequest{
		Nodes: []pluginapi.NodeClaim{
			{
				ID:         pluginapi.NodeID{Kind: "model", Path: "mock/model"},
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

func TestRegisterAndConnectPlugin(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer lis.Close()

	s := grpc.NewServer()
	proto.RegisterCatalogPluginServer(s, &pluginapi.GRPCServer{Impl: mockPlugin{}})

	go func() {
		_ = s.Serve(lis)
	}()
	defer s.Stop()

	addr := lis.Addr().String()
	const pluginName = "mock-external-plugin"

	registered, cleanup, err := Register(map[string]string{pluginName: addr}, nil)
	require.NoError(t, err)
	defer cleanup()

	require.Len(t, registered, 1)

	p, ok := registered[pluginName]
	require.True(t, ok, "plugin %q must be present in the registered map", pluginName)

	req, err := p.Collect(t.Context())
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
