import {
  buildEqualityFilter,
  fetchNodeByName,
  fetchRelations,
  type NodeResource,
  nodeProps,
  type RelationResource,
} from './catalogApi';
import { displayName } from './kindUtils';

export interface CatalogGraphRoot {
  name: string;
}

export interface CatalogGraphNode {
  id: string;
  name: string;
  kind: string;
  path: string;
  label: string;
  depth: number;
  isRoot: boolean;
  properties?: Record<string, unknown>;
}

export interface CatalogGraphEdge {
  id: string;
  kind: string;
  fromNode: string;
  toNode: string;
  direction: 'incoming' | 'outgoing';
}

export interface CatalogGraphResponse {
  nodes: CatalogGraphNode[];
  edges: CatalogGraphEdge[];
}

type FrontierNode = {
  name: string;
  depth: number;
  direction: 'incoming' | 'outgoing';
};

function toGraphNode(node: NodeResource, depth: number, isRoot = false): CatalogGraphNode {
  const properties = nodeProps(node);
  return {
    id: node.name,
    name: node.name,
    kind: node.kind,
    path: node.path,
    label: displayName(node.kind, node.path, properties),
    depth,
    isRoot,
    properties,
  };
}

async function fetchNeighborRelations(token: string | null, node: FrontierNode) {
  if (node.direction === 'outgoing') {
    return fetchRelations(token, {
      filter: buildEqualityFilter('fromNode', node.name),
      pageSize: 1000,
    });
  }

  return fetchRelations(token, {
    filter: buildEqualityFilter('toNode', node.name),
    pageSize: 1000,
  });
}

function buildEdgeFromRelation(
  relation: RelationResource,
  direction: 'incoming' | 'outgoing',
): CatalogGraphEdge {
  return {
    id: `${direction}:${relation.name}`,
    kind: relation.kind,
    fromNode: relation.fromNode,
    toNode: relation.toNode,
    direction,
  };
}

function buildNextFrontierNode(
  relation: RelationResource,
  direction: 'incoming' | 'outgoing',
): FrontierNode | null {
  if (direction === 'outgoing') {
    if (!relation.toNode) {
      return null;
    }

    return {
      name: relation.toNode,
      depth: 0,
      direction,
    };
  }

  if (!relation.fromNode) {
    return null;
  }

  return {
    name: relation.fromNode,
    depth: 0,
    direction,
  };
}

export async function buildCatalogGraphSlice(
  token: string | null,
  root: CatalogGraphRoot,
  maxDepth: number,
): Promise<CatalogGraphResponse> {
  const rootResource = await fetchNodeByName(token, root.name);
  const nodes = new Map<string, CatalogGraphNode>();
  const edges = new Map<string, CatalogGraphEdge>();
  const expanded = new Set<string>();

  nodes.set(rootResource.name, toGraphNode(rootResource, 0, true));

  let frontier: FrontierNode[] = [
    { name: rootResource.name, depth: 0, direction: 'outgoing' },
    { name: rootResource.name, depth: 0, direction: 'incoming' },
  ];

  while (frontier.length > 0) {
    const currentLayer = frontier;
    frontier = [];

    const relationLayers = await Promise.all(
      currentLayer.map(async (currentNode) => {
        const expansionKey = `${currentNode.direction}:${currentNode.name}:${Math.abs(currentNode.depth)}`;
        if (expanded.has(expansionKey) || Math.abs(currentNode.depth) >= maxDepth) {
          return { currentNode, relations: [] as RelationResource[] };
        }

        expanded.add(expansionKey);
        const relations = await fetchNeighborRelations(token, currentNode);
        return { currentNode, relations };
      }),
    );

    const nextTargets = new Map<string, FrontierNode>();

    for (const { currentNode, relations } of relationLayers) {
      for (const relation of relations) {
        const edge = buildEdgeFromRelation(relation, currentNode.direction);
        if (!edge.fromNode || !edge.toNode) {
          continue;
        }

        edges.set(edge.id, edge);

        const nextNode = buildNextFrontierNode(relation, currentNode.direction);
        if (!nextNode) {
          continue;
        }

        nextNode.depth =
          currentNode.direction === 'outgoing' ? currentNode.depth + 1 : currentNode.depth - 1;

        const nextKey = nextNode.name;
        if (!nodes.has(nextKey) && !nextTargets.has(nextKey)) {
          nextTargets.set(nextKey, nextNode);
        }
      }
    }

    const fetchedTargets = await Promise.all(
      Array.from(nextTargets.values()).map(async (target) => {
        const node = await fetchNodeByName(token, target.name);
        return { node, target };
      }),
    );

    for (const { node, target } of fetchedTargets) {
      const key = node.name;
      if (!nodes.has(key)) {
        nodes.set(key, toGraphNode(node, target.depth));
        frontier.push(target);
      }
    }
  }

  return {
    nodes: Array.from(nodes.values()),
    edges: Array.from(edges.values()),
  };
}
