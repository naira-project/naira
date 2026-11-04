import { useQuery } from '@tanstack/react-query';
import { queryKeys } from '../lib/queryKeys';
import { fetchRuntimeConfig, type RuntimeConfig } from '../lib/runtimeConfig';
import { useOpenMFPContext } from './useOpenMFPContext';

/** The deployment's runtime configuration; empty until loaded. */
export function useRuntimeConfig(): RuntimeConfig {
  const { token } = useOpenMFPContext();
  
  const { data = {} } = useQuery({
    queryKey: queryKeys.runtimeConfig(token),
    queryFn: () => fetchRuntimeConfig(token),
    staleTime: Infinity,
  });
  return data;
}
