import { useState, useCallback } from 'react';

interface UseCatalogSyncResult {
  syncing: boolean;
  syncMessage: string | null;
  syncError: string | null;
  handleSync: () => Promise<void>;
}

/**
 * Manages the catalog synchronization workflow:
 * - POSTs to /v1/plugins:run to trigger plugin execution
 * - Parses the response for success/error counts
 * - Exposes loading state, success message, and error message
 * - Calls `onSuccess` callback when sync completes successfully
 */
export function useCatalogSync(token: string | null, onSuccess?: () => void): UseCatalogSyncResult {
  const [syncing, setSyncing] = useState(false);
  const [syncMessage, setSyncMessage] = useState<string | null>(null);
  const [syncError, setSyncError] = useState<string | null>(null);

  const handleSync = useCallback(async () => {
    setSyncing(true);
    setSyncMessage(null);
    setSyncError(null);

    try {
      const response = await fetch('/v1/plugins:run', {
        method: 'POST',
        headers: token ? { Authorization: `Bearer ${token}` } : {},
      });
      const payload = await response.json();

      if (!response.ok) {
        throw new Error(payload.error ?? 'Failed to synchronize data');
      }

      const results = Array.isArray(payload.results) ? payload.results : [];
      const errorCount = results.filter(
        (result: { error?: string }) =>
          typeof result.error === 'string' && result.error.length > 0
      ).length;
      const successCount = results.length - errorCount;
      setSyncMessage(
        errorCount > 0
          ? `Synced ${successCount} plugin(s), ${errorCount} error(s)`
          : `Synced ${successCount} plugin(s)`
      );

      onSuccess?.();
    } catch (error) {
      setSyncError(
        error instanceof Error ? error.message : 'Failed to synchronize data'
      );
    } finally {
      setSyncing(false);
    }
  }, [token, onSuccess]);

  return { syncing, syncMessage, syncError, handleSync };
}