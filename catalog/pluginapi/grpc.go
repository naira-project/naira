package pluginapi

import (
	"context"
	"fmt"

	"github.com/naira-project/naira/catalog/pluginapi/proto"
)

type GRPCServer struct {
	proto.UnimplementedCatalogPluginServer
	Impl Plugin
}

func (s *GRPCServer) Collect(ctx context.Context, _ *proto.Empty) (*proto.IngestionRequest, error) {
	req, err := s.Impl.Collect(ctx)
	if err != nil {
		return nil, fmt.Errorf("collecting from plugin implementation: %w", err)
	}
	return toProtoIngestionRequest(req), nil
}

type GRPCClient struct {
	client proto.CatalogPluginClient
}

func NewGRPCClient(client proto.CatalogPluginClient) *GRPCClient {
	return &GRPCClient{client: client}
}

func (c *GRPCClient) Collect(ctx context.Context) (IngestionRequest, error) {
	resp, err := c.client.Collect(ctx, &proto.Empty{})
	if err != nil {
		return IngestionRequest{}, fmt.Errorf("calling gRPC Collect: %w", err)
	}
	return fromProtoIngestionRequest(resp), nil
}
