import { useQuery } from '@tanstack/react-query';
import {
  buildEqualityFilter,
  fetchNode,
  fetchNodeByName,
  fetchRelations,
  NodeResource,
} from '../lib/catalogApi';
import { queryKeys } from '../lib/queryKeys';
import { useOpenMFPContext } from './useOpenMFPContext';

/**
 * Generic hook to fetch a single catalog node by kind + path.
 */
export function useCatalogDetail(kind: string, path: string) {
  const { token } = useOpenMFPContext();
  const {
    data: node = null,
    isLoading: loading,
    error,
  } = useQuery({
    queryKey: queryKeys.node(kind, path),
    queryFn: () => fetchNode(token, kind, path),
    enabled: kind.length > 0 && path.length > 0,
  });

  return {
    node,
    loading,
    error: error ? `Failed to load ${kind} "${path}"` : null,
  };
}
