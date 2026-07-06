import { useCallback, useEffect, useState } from 'react';
import { buildCatalogGraphSlice, type CatalogGraphResponse, type CatalogGraphRoot } from '../lib/catalogGraph';
import { useOpenMFPContext } from './useOpenMFPContext';

export type { CatalogGraphEdge, CatalogGraphNode, CatalogGraphRoot, CatalogGraphResponse } from '../lib/catalogGraph';

export function useCatalogGraph(root: CatalogGraphRoot | null, depth: number) {
  const { token, isReady } = useOpenMFPContext();
  const [graph, setGraph] = useState<CatalogGraphResponse>({ nodes: [], edges: [] });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const loadGraph = useCallback(() => {
    if (!root || !isReady) {
      setGraph({ nodes: [], edges: [] });
      setLoading(!isReady);
      setError(null);
      return;
    }

    setLoading(true);
    setError(null);

    buildCatalogGraphSlice(token, root, depth)
      .then((nextGraph) => {
        setGraph(nextGraph);
        setLoading(false);
      })
      .catch(() => {
        setError('Failed to load graph');
        setLoading(false);
      });
  }, [depth, root, token, isReady]);

  useEffect(() => {
    loadGraph();
  }, [loadGraph]);

  return { graph, loading, error, reload: loadGraph };
}