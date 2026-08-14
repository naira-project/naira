import { useQuery } from '@tanstack/react-query';
import {
  buildEqualityFilter,
  fetchNode,
  fetchNodeByName,
  fetchRelations,
  NodeResource,
} from '../lib/catalogApi';
import { queryKeys } from '../lib/queryKeys';

/**
 * Generic hook to fetch a single catalog node by kind + path.
 * Shares its cache entry with anything else that reads the same node
 * (e.g. useCatalogGraph resolving the same node by name).
 */
export function useCatalogDetail(kind: string, path: string) {
  const {
    data: node = null,
    isLoading: loading,
    error,
  } = useQuery({
    queryKey: queryKeys.node(kind, path),
    queryFn: () => fetchNode(kind, path),
    enabled: kind.length > 0 && path.length > 0,
  });

  return {
    node,
    loading,
    error: error ? `Failed to load ${kind} "${path}"` : null,
  };
}

/**
 * Fetch related "from" nodes for the given node name.
 * Used to resolve adopters / dependants. Kept as a plain async helper
 * (not a query hook) since it's invoked imperatively/on demand rather
 * than being tied to a component's render lifecycle.
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
