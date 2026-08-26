import { useQuery } from '@tanstack/react-query';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { discoverKinds } from '../lib/kindUtils';
import { queryKeys } from '../lib/queryKeys';
import { useOpenMFPContext } from './useOpenMFPContext';

interface UseKindsResult {
  kinds: string[];
  kindsLoading: boolean;
  kindsError: string | null;
  activeKind: string | null;
  setActiveKind: (kind: string | null) => void;
  refreshKinds: () => void;
}

/**
 * Encapsulates kind discovery, loading/error state, active kind selection,
 * and a refresh mechanism.
 */
export function useKinds(viewpointKinds?: string[]): UseKindsResult {
  const [activeKind, setActiveKind] = useState<string | null>(null);
  const { token } = useOpenMFPContext();

  const {
    data: discoveredKinds = [],
    isLoading: kindsLoading,
    error,
    refetch,
  } = useQuery({
    queryKey: queryKeys.kinds,
    queryFn: () => discoverKinds(token),
  });

  const kinds = useMemo(
    () =>
      viewpointKinds ? viewpointKinds.filter((k) => discoveredKinds.includes(k)) : discoveredKinds,
    [viewpointKinds?.join(','), discoveredKinds],
  );

  // Auto-select first kind if none selected yet.
  useEffect(() => {
    setActiveKind((prev) => (prev === null && kinds.length > 0 ? kinds[0] : prev));
  }, [kinds]);

  const refreshKinds = useCallback(() => {
    refetch();
  }, [refetch]);

  return {
    kinds,
    kindsLoading,
    kindsError: error ? 'Failed to discover kinds' : null,
    activeKind,
    setActiveKind,
    refreshKinds,
  };
}
