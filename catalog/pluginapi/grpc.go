package pluginapi

import (
	"context"
	"fmt"

	pluginv1 "github.com/naira-project/naira/catalog/pluginapi/proto/plugin/v1"
)

type GRPCServer struct {
	pluginv1.UnimplementedCatalogPluginServer
	Impl Plugin
}

func (s *GRPCServer) Collect(ctx context.Context, _ *pluginv1.Empty) (*pluginv1.IngestionRequest, error) {
	req, err := s.Impl.Collect(ctx)
	if err != nil {
		return nil, fmt.Errorf("collecting from plugin implementation: %w", err)
	}
	return toProtoIngestionRequest(req), nil
}

type GRPCClient struct {
	client pluginv1.CatalogPluginClient
}

func NewGRPCClient(client pluginv1.CatalogPluginClient) *GRPCClient {
	return &GRPCClient{client: client}
}

func (c *GRPCClient) Collect(ctx context.Context) (IngestionRequest, error) {
	resp, err := c.client.Collect(ctx, &pluginv1.Empty{})
	if err != nil {
		return IngestionRequest{}, fmt.Errorf("calling gRPC Collect: %w", err)
	}
	return fromProtoIngestionRequest(resp), nil
}
