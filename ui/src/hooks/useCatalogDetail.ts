import {
  buildEqualityFilter,
  fetchNode,
  fetchNodeByName,
  fetchRelations,
  NodeResource,
} from '../lib/catalogApi';
import { useAsyncData } from './useAsyncData';
import { useOpenMFPContext } from './useOpenMFPContext';

interface CatalogDetailResult {
  node: NodeResource | null;
  loading: boolean;
  error: string | null;
}

/**
 * Generic hook to fetch a single catalog node by kind + path.
 */
export function useCatalogDetail(kind: string, path: string) {
  const { token } = useOpenMFPContext();

  const { data: node, loading, error } = useAsyncData<NodeResource | null>(
    () => fetchNode(token, kind, path),
    [kind, path, token],
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
  token: string | null,
  nodeName: string,
  relationKind?: string
): Promise<NodeResource[]> {
  const relations = await fetchRelations(token, {
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
        return await fetchNodeByName(token, name);
      } catch {
        return null;
      }
    })
  );

  return nodes.filter((n): n is NodeResource => n !== null);
}
