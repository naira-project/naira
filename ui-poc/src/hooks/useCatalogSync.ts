import { useMutation, useQueryClient } from '@tanstack/react-query';
import { queryKeys } from '../lib/queryKeys';

interface SyncPayload {
  results?: { error?: string }[];
  error?: string;
}

async function triggerSync(): Promise<string> {
  const response = await fetch('/v1/plugins:run', { method: 'POST' });
  const payload = (await response.json()) as SyncPayload;

  if (!response.ok) {
    throw new Error(payload.error ?? 'Failed to synchronize data');
  }

  const results = Array.isArray(payload.results) ? payload.results : [];
  const errorCount = results.filter(
    (result) => typeof result.error === 'string' && result.error.length > 0
  ).length;
  const successCount = results.length - errorCount;

  return errorCount > 0
    ? `Synced ${successCount} plugin(s), ${errorCount} error(s)`
    : `Synced ${successCount} plugin(s)`;
}

interface UseCatalogSyncResult {
  syncing: boolean;
  syncMessage: string | null;
  syncError: string | null;
  handleSync: () => Promise<void>;
}

/**
 * Manages the catalog synchronization workflow via a mutation:
 * - POSTs to /v1/plugins:run to trigger plugin execution
 * - On success, invalidates operations + kinds so dependent queries refetch
 * - Calls `onSuccess` callback when sync completes successfully
 */
export function useCatalogSync(onSuccess?: () => void): UseCatalogSyncResult {
  const queryClient = useQueryClient();

  const mutation = useMutation({
    mutationFn: triggerSync,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.operations });
      queryClient.invalidateQueries({ queryKey: queryKeys.kinds });
      onSuccess?.();
    },
  });

  const handleSync = async () => {
    await mutation.mutateAsync().catch(() => {});
  };

  return {
    syncing: mutation.isPending,
    syncMessage: mutation.data ?? null,
    syncError:
      mutation.error instanceof Error
        ? mutation.error.message
        : mutation.error
        ? 'Failed to synchronize data'
        : null,
    handleSync,
  };
}
