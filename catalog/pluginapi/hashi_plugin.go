package pluginapi

import (
	"context"
	"errors"
	"fmt"
	"net/rpc"

	"github.com/hashicorp/go-plugin"
	"github.com/naira-project/naira/catalog/pluginapi/proto"
	"google.golang.org/grpc"
)

// HandshakeConfig prevents random binaries from connecting to Naira Catalog.
var HandshakeConfig = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "CATALOG_PLUGIN_MAGIC_COOKIE",
	MagicCookieValue: "naira-catalog-system",
}

type HashiPlugin struct {
	Impl Plugin
}

func (p *HashiPlugin) Server(*plugin.MuxBroker) (interface{}, error) {
	return nil, errors.New("net/rpc is not supported, use gRPC")
}

func (p *HashiPlugin) Client(*plugin.MuxBroker, *rpc.Client) (interface{}, error) {
	return nil, errors.New("net/rpc is not supported, use gRPC")
}

func (p *HashiPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	proto.RegisterCatalogPluginServer(s, &GRPCServer{Impl: p.Impl})
	return nil
}

func (p *HashiPlugin) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &GRPCClient{client: proto.NewCatalogPluginClient(c)}, nil
}

type GRPCServer struct {
	proto.UnimplementedCatalogPluginServer
	Impl Plugin
}

func (s *GRPCServer) Name(ctx context.Context, _ *proto.Empty) (*proto.NameResponse, error) {
	return &proto.NameResponse{Name: s.Impl.Name()}, nil
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

func (c *GRPCClient) Name() string {
	resp, err := c.client.Name(context.Background(), &proto.Empty{})
	if err != nil {
		return ""
	}
	return resp.Name
}

func (c *GRPCClient) Collect(ctx context.Context) (IngestionRequest, error) {
	resp, err := c.client.Collect(ctx, &proto.Empty{})
	if err != nil {
		return IngestionRequest{}, fmt.Errorf("calling gRPC Collect: %w", err)
	}
	return fromProtoIngestionRequest(resp), nil
}
