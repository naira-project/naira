import {
  buildEqualityFilter,
  fetchNode,
  fetchNodeByName,
  fetchRelations,
  NodeResource,
} from '../lib/catalogApi';
import { useAsyncData } from './useAsyncData';

interface CatalogDetailResult {
  node: NodeResource | null;
  loading: boolean;
  error: string | null;
}

/**
 * Generic hook to fetch a single catalog node by kind + path.
 */
export function useCatalogDetail(kind: string, path: string) {
  const { data: node, loading, error } = useAsyncData<NodeResource | null>(
    () => fetchNode(kind, path),
    [kind, path],
    null,
    `Failed to load ${kind} "${path}"`
  );

  return { node, loading, error };
}

/**
 * Fetch related "from" nodes for the given node name.
 * Used to resolve adopters / dependants.
 */
export async function fetchRelatedNodes(
  nodeName: string,
  relationKind?: string
): Promise<NodeResource[]> {
  const relations = await fetchRelations({
    filter: buildEqualityFilter('toNode', nodeName),
    pageSize: 1000,
  });

  const filtered = relationKind
    ? relations.filter((r) => r.kind === relationKind)
    : relations;

  const names = filtered.map((r) => r.fromNode).filter(Boolean) as string[];

  const nodes = await Promise.all(
    names.map(async (name) => {
      try {
        return await fetchNodeByName(name);
      } catch {
        return null;
      }
    })
  );

  return nodes.filter((n): n is NodeResource => n !== null);
}
