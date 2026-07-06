import { useState, useEffect } from 'react';
import {
  buildEqualityFilter,
  fetchNodes,
  NodeResource,
} from '../lib/catalogApi';

interface CatalogNodesResult {
  nodes: NodeResource[];
  loading: boolean;
  error: string | null;
}

/**
 * Generic hook to fetch all catalog nodes of a given kind.
 * Re-fetches when `kind` changes.
 */
export function useCatalogNodes(kind: string): CatalogNodesResult {
  const [nodes, setNodes] = useState<NodeResource[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setLoading(true);
    setError(null);

    fetchNodes({
      filter: buildEqualityFilter('kind', kind),
      pageSize: 1000,
    })
      .then((result) => {
        setNodes(result);
        setLoading(false);
      })
      .catch(() => {
        setError(`Failed to load nodes of kind "${kind}"`);
        setLoading(false);
      });
  }, [kind]);

  return { nodes, loading, error };
}
