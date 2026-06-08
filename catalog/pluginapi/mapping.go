package pluginapi

import "github.com/naira-project/naira/catalog/pluginapi/proto"

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
