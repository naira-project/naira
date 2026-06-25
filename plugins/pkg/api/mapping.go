package pluginapi

import pluginv1 "github.com/naira-project/naira/plugins/pkg/api/proto/plugin/v1"

func toProtoNodeID(id NodeID) *pluginv1.NodeID {
	return &pluginv1.NodeID{
		Kind: id.Kind,
		Path: id.Path,
	}
}

func fromProtoNodeID(p *pluginv1.NodeID) NodeID {
	if p == nil {
		return NodeID{}
	}
	return NodeID{
		Kind: p.Kind,
		Path: p.Path,
	}
}

func toProtoPropertyMap(properties PropertyMap) map[string]string {
	return map[string]string(properties)
}

func fromProtoPropertyMap(p map[string]string) PropertyMap {
	return PropertyMap(p)
}

func toProtoNodeClaim(claim NodeClaim) *pluginv1.NodeClaim {
	return &pluginv1.NodeClaim{
		Id:         toProtoNodeID(claim.ID),
		Properties: toProtoPropertyMap(claim.Properties),
	}
}

func fromProtoNodeClaim(p *pluginv1.NodeClaim) NodeClaim {
	if p == nil {
		return NodeClaim{}
	}
	return NodeClaim{
		ID:         fromProtoNodeID(p.Id),
		Properties: fromProtoPropertyMap(p.Properties),
	}
}

func toProtoRelationClaim(claim RelationClaim) *pluginv1.RelationClaim {
	return &pluginv1.RelationClaim{
		Kind:       claim.Kind,
		From:       toProtoNodeID(claim.From),
		To:         toProtoNodeID(claim.To),
		Properties: toProtoPropertyMap(claim.Properties),
	}
}

func fromProtoRelationClaim(p *pluginv1.RelationClaim) RelationClaim {
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

func toProtoCollectResponse(req CollectResponse) *pluginv1.CollectResponse {
	nodes := make([]*pluginv1.NodeClaim, len(req.Nodes))
	for i, node := range req.Nodes {
		nodes[i] = toProtoNodeClaim(node)
	}
	relations := make([]*pluginv1.RelationClaim, len(req.Relations))
	for i, rel := range req.Relations {
		relations[i] = toProtoRelationClaim(rel)
	}
	return &pluginv1.CollectResponse{
		Nodes:     nodes,
		Relations: relations,
	}
}

func fromProtoCollectResponse(p *pluginv1.CollectResponse) CollectResponse {
	if p == nil {
		return CollectResponse{}
	}
	nodes := make([]NodeClaim, len(p.Nodes))
	for i, node := range p.Nodes {
		nodes[i] = fromProtoNodeClaim(node)
	}
	relations := make([]RelationClaim, len(p.Relations))
	for i, rel := range p.Relations {
		relations[i] = fromProtoRelationClaim(rel)
	}
	return CollectResponse{
		Nodes:     nodes,
		Relations: relations,
	}
}
