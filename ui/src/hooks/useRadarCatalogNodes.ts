import { useMemo } from 'react';
import type { NodeResource } from '../lib/catalogApi';
import { useCatalogNodes } from './useCatalogNodes';

export interface RadarCatalogNodes {
  /** Where a model is available. */
  models: NodeResource[];
  /** Where a model is currently served. */
  endpoints: NodeResource[];
  loading: boolean;
  error: string | null;
}

/**
 * Fetches the catalog nodes that radar entries can link to via their catalog
 * refs: models and inference endpoints.
 */
export function useRadarCatalogNodes(): RadarCatalogNodes {
  const models = useCatalogNodes('model');
  const endpoints = useCatalogNodes('inference_endpoint');

  const loading = models.loading || endpoints.loading;
  const error = models.error || endpoints.error ? 'Failed to load catalog matches' : null;

  return useMemo(
    () => ({ models: models.nodes, endpoints: endpoints.nodes, loading, error }),
    [models.nodes, endpoints.nodes, loading, error],
  );
}
