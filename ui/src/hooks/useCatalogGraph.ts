import { useQuery } from '@tanstack/react-query';
import {
  buildCatalogGraphSlice,
  type CatalogGraphResponse,
  type CatalogGraphRoot,
} from '../lib/catalogGraph';
import { queryKeys } from '../lib/queryKeys';
import { useOpenMFPContext } from './useOpenMFPContext';

export type {
  CatalogGraphEdge,
  CatalogGraphNode,
  CatalogGraphResponse,
  CatalogGraphRoot,
} from '../lib/catalogGraph';

const EMPTY_GRAPH: CatalogGraphResponse = { nodes: [], edges: [] };

export function useCatalogGraph(root: CatalogGraphRoot | null, depth: number) {
  const { token } = useOpenMFPContext();
  const {
    data: graph = EMPTY_GRAPH,
    isLoading: loading,
    error,
  } = useQuery({
    queryKey: queryKeys.graph(root?.name ?? null, depth),
    queryFn: () => buildCatalogGraphSlice(token, root as CatalogGraphRoot, depth),
    enabled: root !== null,
  });

  return { graph, loading, error: error ? 'Failed to load graph' : null };
}
