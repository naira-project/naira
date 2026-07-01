package pluginapi

import (
	"context"
	"fmt"

	pluginv1 "github.com/naira-project/naira/plugins/pkg/pluginapi/proto/plugin/v1"
)

type GRPCServer struct {
	pluginv1.UnimplementedCatalogPluginServiceServer
	Impl Plugin
}

func (s *GRPCServer) Collect(ctx context.Context, _ *pluginv1.CollectRequest) (*pluginv1.CollectResponse, error) {
	if s.Impl == nil {
		return nil, fmt.Errorf("plugin implementation is missing")
	}
	req, err := s.Impl.Collect(ctx)
	if err != nil {
		return nil, fmt.Errorf("collecting from plugin implementation: %w", err)
	}
	return toProtoCollectResponse(req), nil
}

type GRPCClient struct {
	Client pluginv1.CatalogPluginServiceClient
}

func (c *GRPCClient) Collect(ctx context.Context) (CollectResponse, error) {
	if c.Client == nil {
		return CollectResponse{}, fmt.Errorf("gRPC client connection is missing")
	}
	resp, err := c.Client.Collect(ctx, &pluginv1.CollectRequest{})
	if err != nil {
		return CollectResponse{}, fmt.Errorf("calling gRPC Collect: %w", err)
	}
	return fromProtoCollectResponse(resp), nil
}
