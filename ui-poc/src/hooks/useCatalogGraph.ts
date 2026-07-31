import { buildCatalogGraphSlice, type CatalogGraphResponse, type CatalogGraphRoot } from '../lib/catalogGraph';
import { useOpenMFPContext } from './useOpenMFPContext';
import { useAsyncData } from './useAsyncData';

export type { CatalogGraphEdge, CatalogGraphNode, CatalogGraphRoot, CatalogGraphResponse } from '../lib/catalogGraph';

const EMPTY_GRAPH: CatalogGraphResponse = { nodes: [], edges: [] };

export function useCatalogGraph(root: CatalogGraphRoot | null, depth: number) {
  const { token } = useOpenMFPContext();

  const { data: graph, loading, error } = useAsyncData<CatalogGraphResponse>(
    () => (root ? buildCatalogGraphSlice(token, root, depth) : Promise.resolve(EMPTY_GRAPH)),
    [root, depth, token],
    EMPTY_GRAPH,
    'Failed to load graph'
  );

  return { graph, loading, error };
}