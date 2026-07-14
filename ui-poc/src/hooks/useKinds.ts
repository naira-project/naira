import { useState, useEffect, useCallback } from 'react';
import { discoverKinds } from '../lib/kindUtils';

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
 *
 * Automatically discovers kinds on mount and selects the first kind by default.
 */
export function useKinds(): UseKindsResult {
  const [kinds, setKinds] = useState<string[]>([]);
  const [kindsLoading, setKindsLoading] = useState(true);
  const [kindsError, setKindsError] = useState<string | null>(null);
  const [activeKind, setActiveKind] = useState<string | null>(null);

  const loadKinds = useCallback(() => {
    setKindsLoading(true);
    setKindsError(null);
    discoverKinds()
      .then((result) => {
        setKinds(result);
        // Auto-select first kind if none selected
        setActiveKind((prev) => (prev === null && result.length > 0 ? result[0] : prev));
        setKindsLoading(false);
      })
      .catch(() => {
        setKindsError('Failed to discover kinds');
        setKindsLoading(false);
      });
  }, []);

  useEffect(() => {
    loadKinds();
  }, [loadKinds]);

  return {
    kinds,
    kindsLoading,
    kindsError,
    activeKind,
    setActiveKind,
    refreshKinds: loadKinds,
  };
}