package pluginmanager

import (
	"context"
	"net"
	"testing"

	"github.com/naira-project/naira/catalog/pluginapi"
	pluginv1 "github.com/naira-project/naira/catalog/pluginapi/proto/plugin/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type mockPlugin struct{}

var stubResponse = pluginapi.CollectResponse{
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
}

func (mockPlugin) Collect(ctx context.Context) (pluginapi.CollectResponse, error) {
	return stubResponse, nil
}

func TestRegisterAndConnectPlugin(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer lis.Close()

	s := grpc.NewServer()
	pluginv1.RegisterCatalogPluginServiceServer(s, &pluginapi.GRPCServer{Impl: mockPlugin{}})

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

	assert.Equal(t, stubResponse, req)
}
