import { useCallback, useEffect, useState } from 'react';
import { buildCatalogGraphSlice, type CatalogGraphResponse, type CatalogGraphRoot } from '../lib/catalogGraph';

export type { CatalogGraphEdge, CatalogGraphNode, CatalogGraphRoot, CatalogGraphResponse } from '../lib/catalogGraph';

export function useCatalogGraph(root: CatalogGraphRoot | null, depth: number) {
  const [graph, setGraph] = useState<CatalogGraphResponse>({ nodes: [], edges: [] });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const loadGraph = useCallback(() => {
    if (!root) {
      setGraph({ nodes: [], edges: [] });
      setLoading(false);
      setError(null);
      return;
    }

    setLoading(true);
    setError(null);

    buildCatalogGraphSlice(root, depth)
      .then((nextGraph) => {
        setGraph(nextGraph);
        setLoading(false);
      })
      .catch(() => {
        setError('Failed to load graph');
        setLoading(false);
      });
  }, [depth, root]);

  useEffect(() => {
    loadGraph();
  }, [loadGraph]);

  return { graph, loading, error, reload: loadGraph };
}