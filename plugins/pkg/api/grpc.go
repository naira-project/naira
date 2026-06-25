package pluginapi

import (
	"context"
	"fmt"

	pluginv1 "github.com/naira-project/naira/plugins/pkg/api/proto/plugin/v1"
)

type GRPCServer struct {
	pluginv1.UnimplementedCatalogPluginServiceServer
	Impl Plugin
}

func (s *GRPCServer) Collect(ctx context.Context, _ *pluginv1.CollectRequest) (*pluginv1.CollectResponse, error) {
	req, err := s.Impl.Collect(ctx)
	if err != nil {
		return nil, fmt.Errorf("collecting from plugin implementation: %w", err)
	}
	return toProtoCollectResponse(req), nil
}

type GRPCClient struct {
	client pluginv1.CatalogPluginServiceClient
}

func NewGRPCClient(client pluginv1.CatalogPluginServiceClient) *GRPCClient {
	return &GRPCClient{client: client}
}

func (c *GRPCClient) Collect(ctx context.Context) (CollectResponse, error) {
	resp, err := c.client.Collect(ctx, &pluginv1.CollectRequest{})
	if err != nil {
		return CollectResponse{}, fmt.Errorf("calling gRPC Collect: %w", err)
	}
	return fromProtoCollectResponse(resp), nil
}
