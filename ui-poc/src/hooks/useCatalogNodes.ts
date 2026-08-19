import { useQuery } from '@tanstack/react-query';
import {
  buildEqualityFilter,
  fetchNodes,
  NodeResource,
} from '../lib/catalogApi';
import { queryKeys } from '../lib/queryKeys';
import { useOpenMFPContext } from './useOpenMFPContext';

interface CatalogNodesResult {
  nodes: NodeResource[];
  loading: boolean;
  error: string | null;
}

/**
 * Generic hook to fetch all catalog nodes of a given kind.
 */
export function useCatalogNodes(kind: string): CatalogNodesResult {
  const { token } = useOpenMFPContext();

  const {
    data: nodes = [],
    isLoading,
    error,
  } = useQuery({
    queryKey: queryKeys.nodes(kind),
    queryFn: () => fetchNodes(token, { filter: buildEqualityFilter('kind', kind), pageSize: 1000 }),
    enabled: kind.length > 0,
  });

  return {
    nodes,
    loading: isLoading,
    error: error ? `Failed to load nodes of kind "${kind}"` : null,
  };
}
