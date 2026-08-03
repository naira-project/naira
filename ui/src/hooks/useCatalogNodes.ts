import {
  buildEqualityFilter,
  fetchNodes,
  NodeResource,
} from '../lib/catalogApi';
import { useAsyncData } from './useAsyncData';

interface CatalogNodesResult {
  nodes: NodeResource[];
  loading: boolean;
  error: string | null;
}

/**
 * Generic hook to fetch all catalog nodes of a given kind.
 * Re-fetches when `kind` changes.
 */
export function useCatalogNodes(kind: string) {
  const { data: nodes, loading, error } = useAsyncData<NodeResource[]>(
    () => fetchNodes({ filter: buildEqualityFilter('kind', kind), pageSize: 1000 }),
    [kind],
    [],
    `Failed to load nodes of kind "${kind}"`
  );

  return { nodes, loading, error };
}