import { useMemo } from 'react';
import {
  parseRadarModel,
  type RadarModel,
  TECH_RADAR_ENTRY_KIND,
  TECH_RADAR_KIND,
} from '../lib/techRadar';
import { useCatalogNodes } from './useCatalogNodes';

interface TechRadarResult {
  model: RadarModel | null;
  loading: boolean;
  error: string | null;
}

/**
 * Fetches the tech_radar node and its entries, assembled into a view model.
 * A null model with loading=false means no radar has been synced yet.
 */
export function useTechRadar(): TechRadarResult {
  const radar = useCatalogNodes(TECH_RADAR_KIND);
  const entries = useCatalogNodes(TECH_RADAR_ENTRY_KIND);

  const model = useMemo(
    () => parseRadarModel(radar.nodes[0], entries.nodes),
    [radar.nodes, entries.nodes],
  );

  return {
    model,
    loading: radar.loading || entries.loading,
    error: radar.error || entries.error ? 'Failed to load the tech radar' : null,
  };
}
