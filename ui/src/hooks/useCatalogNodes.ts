import {
  buildEqualityFilter,
  fetchNodes,
  NodeResource,
} from '../lib/catalogApi';
import { useAsyncData } from './useAsyncData';
import { useOpenMFPContext } from './useOpenMFPContext';

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
  const { token } = useOpenMFPContext();

  const { data: nodes, loading, error } = useAsyncData<NodeResource[]>(
    () => fetchNodes(token, { filter: buildEqualityFilter('kind', kind), pageSize: 1000 }),
    [kind, token],
    [],
    `Failed to load nodes of kind "${kind}"`
  );

  return { nodes, loading, error };
}