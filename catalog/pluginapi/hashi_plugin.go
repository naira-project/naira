package pluginapi

import (
	"context"
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
	return nil, nil
}

func (p *HashiPlugin) Client(*plugin.MuxBroker, *rpc.Client) (interface{}, error) {
	return nil, nil
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

func toProtoNodeID(id NodeID) *proto.NodeID {
	return &proto.NodeID{
		Kind: id.Kind,
		Path: id.Path,
	}
}

func fromProtoNodeID(p *proto.NodeID) NodeID {
	if p == nil {
		return NodeID{}
	}
	return NodeID{
		Kind: p.Kind,
		Path: p.Path,
	}
}

func toProtoPropertyMap(properties PropertyMap) map[string]string {
	if properties == nil {
		return nil
	}
	return map[string]string(properties)
}

func fromProtoPropertyMap(p map[string]string) PropertyMap {
	if p == nil {
		return nil
	}
	return PropertyMap(p)
}

func toProtoNodeClaim(claim NodeClaim) *proto.NodeClaim {
	return &proto.NodeClaim{
		Id:         toProtoNodeID(claim.ID),
		Properties: toProtoPropertyMap(claim.Properties),
	}
}

func fromProtoNodeClaim(p *proto.NodeClaim) NodeClaim {
	if p == nil {
		return NodeClaim{}
	}
	return NodeClaim{
		ID:         fromProtoNodeID(p.Id),
		Properties: fromProtoPropertyMap(p.Properties),
	}
}

func toProtoRelationClaim(claim RelationClaim) *proto.RelationClaim {
	return &proto.RelationClaim{
		Kind:       claim.Kind,
		From:       toProtoNodeID(claim.From),
		To:         toProtoNodeID(claim.To),
		Properties: toProtoPropertyMap(claim.Properties),
	}
}

func fromProtoRelationClaim(p *proto.RelationClaim) RelationClaim {
	if p == nil {
		return RelationClaim{}
	}
	return RelationClaim{
		Kind:       p.Kind,
		From:       fromProtoNodeID(p.From),
		To:         fromProtoNodeID(p.To),
		Properties: fromProtoPropertyMap(p.Properties),
	}
}

func toProtoIngestionRequest(req IngestionRequest) *proto.IngestionRequest {
	nodes := make([]*proto.NodeClaim, len(req.Nodes))
	for i, node := range req.Nodes {
		nodes[i] = toProtoNodeClaim(node)
	}
	relations := make([]*proto.RelationClaim, len(req.Relations))
	for i, rel := range req.Relations {
		relations[i] = toProtoRelationClaim(rel)
	}
	return &proto.IngestionRequest{
		Nodes:     nodes,
		Relations: relations,
	}
}

func fromProtoIngestionRequest(p *proto.IngestionRequest) IngestionRequest {
	if p == nil {
		return IngestionRequest{}
	}
	nodes := make([]NodeClaim, len(p.Nodes))
	for i, node := range p.Nodes {
		nodes[i] = fromProtoNodeClaim(node)
	}
	relations := make([]RelationClaim, len(p.Relations))
	for i, rel := range p.Relations {
		relations[i] = fromProtoRelationClaim(rel)
	}
	return IngestionRequest{
		Nodes:     nodes,
		Relations: relations,
	}
}
