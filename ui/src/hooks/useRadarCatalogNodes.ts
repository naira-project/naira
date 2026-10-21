import { type UseQueryResult, useQueries } from '@tanstack/react-query';
import { buildEqualityFilter, fetchNodes, type NodeResource } from '../lib/catalogApi';
import { queryKeys } from '../lib/queryKeys';
import { useOpenMFPContext } from './useOpenMFPContext';

export interface RadarCatalogNodes {
  /** Catalog nodes of every requested kind. */
  nodes: NodeResource[];
  loading: boolean;
  error: string | null;
}

// Module-level so its reference is stable: useQueries re-runs combine, and
// returns a new object, whenever the function changes.
function combineNodes(results: UseQueryResult<NodeResource[]>[]): RadarCatalogNodes {
  return {
    nodes: results.flatMap((result) => result.data ?? []),
    loading: results.some((result) => result.isLoading),
    error: results.some((result) => result.error) ? 'Failed to load catalog matches' : null,
  };
}

/**
 * Fetches the catalog nodes of the given kinds, the ones radar entries can
 * link to via their catalog refs. Shares its cache with useCatalogNodes.
 */
export function useRadarCatalogNodes(kinds: string[]): RadarCatalogNodes {
  const { token } = useOpenMFPContext();

  return useQueries({
    queries: kinds.map((kind) => ({
      queryKey: queryKeys.nodes(kind),
      queryFn: () =>
        fetchNodes(token, { filter: buildEqualityFilter('kind', kind), pageSize: 1000 }),
    })),
    combine: combineNodes,
  });
}
