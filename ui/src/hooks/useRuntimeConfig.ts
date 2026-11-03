import { useQuery } from '@tanstack/react-query';
import { queryKeys } from '../lib/queryKeys';
import { fetchRuntimeConfig, type RuntimeConfig } from '../lib/runtimeConfig';

/** The deployment's runtime configuration; empty until loaded. */
export function useRuntimeConfig(): RuntimeConfig {
  const { data = {} } = useQuery({
    queryKey: queryKeys.runtimeConfig,
    queryFn: fetchRuntimeConfig,
    staleTime: Infinity,
  });
  return data;
}
