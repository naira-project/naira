import { useQuery } from '@tanstack/react-query';
import { useMemo } from 'react';
import { buildEqualityFilter, fetchNodes } from '../lib/catalogApi';
import { queryKeys } from '../lib/queryKeys';
import {
  parseRadarModel,
  type RadarModel,
  TECH_RADAR_ENTRY_KIND,
  TECH_RADAR_KIND,
} from '../lib/techRadar';
import { useOpenMFPContext } from './useOpenMFPContext';

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
  const { token } = useOpenMFPContext();

  const radarQuery = useQuery({
    queryKey: queryKeys.nodes(TECH_RADAR_KIND),
    queryFn: () =>
      fetchNodes(token, { filter: buildEqualityFilter('kind', TECH_RADAR_KIND), pageSize: 1000 }),
  });
  const entriesQuery = useQuery({
    queryKey: queryKeys.nodes(TECH_RADAR_ENTRY_KIND),
    queryFn: () =>
      fetchNodes(token, {
        filter: buildEqualityFilter('kind', TECH_RADAR_ENTRY_KIND),
        pageSize: 1000,
      }),
  });

  const model = useMemo(
    () => parseRadarModel(radarQuery.data?.[0], entriesQuery.data ?? []),
    [radarQuery.data, entriesQuery.data],
  );

  return {
    model,
    loading: radarQuery.isLoading || entriesQuery.isLoading,
    error: radarQuery.error || entriesQuery.error ? 'Failed to load the tech radar' : null,
  };
}
