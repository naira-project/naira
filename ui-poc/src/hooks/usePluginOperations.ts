import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  OperationResource,
  runAllPlugins as apiRunAllPlugins,
  runPlugin as apiRunPlugin,
  fetchOperations,
  fetchPlugins,
} from '../lib/catalogApi';
import { queryKeys } from '../lib/queryKeys';
import { useOpenMFPContext } from './useOpenMFPContext';

const POLL_INTERVAL_MS = 2000;
const EMPTY_OPERATIONS: OperationResource[] = [];
// How long a non-terminal operation is given before we surface a
// "taking longer than expected" notice. Mirrors the old
// POLL_MAX_ATTEMPTS * POLL_INTERVAL_MS (60 * 2s = 2 minutes), but is now
// evaluated against the operation's own startTime instead of a manual
// client-side attempt counter.
const STALE_AFTER_MS = 2 * 60 * 1000;

const TERMINAL_STATES: OperationResource['state'][] = ['SUCCEEDED', 'FAILED'];

function isTerminal(op: OperationResource) {
  return TERMINAL_STATES.includes(op.state);
}

function isStale(op: OperationResource) {
  if (isTerminal(op)) return false;

  const startTime = new Date(op.startTime).getTime();
  const createdTime = new Date(op.createdAt).getTime();
  const referenceTime = Number.isFinite(startTime) && startTime > 0 ? startTime : createdTime;

  return Number.isFinite(referenceTime) && Date.now() - referenceTime > STALE_AFTER_MS;
}

function mergeOperations(
  current: OperationResource[] = [],
  incoming: OperationResource[]
): OperationResource[] {
  const incomingByName = new Map(incoming.map((op) => [op.name, op]));
  const existing = current.filter((op) => !incomingByName.has(op.name));
  return [...incoming, ...existing];
}

export interface RunErrorEntry {
  id: string;
  message: string;
}

// ---------------------------------------------------------------------------
// usePluginsStatus — single source of truth for the plugins list and their
// run operations. "Run" (per plugin) and "Run All" are fully independent:
// each plugin's running state is derived from the operations list itself,
// so triggering one plugin never disables the button for another.
//
// Polling is now handled by react-query's refetchInterval: as long as any
// operation is non-terminal, /v1/operations is polled every 2s; it stops
// automatically once everything settles. No manual per-operation polling
// loop is needed any more.
// ---------------------------------------------------------------------------

interface UsePluginsStatusResult {
  plugins: string[];
  operations: OperationResource[];
  loading: boolean;
  /** Plugin names with at least one in-flight run (via "Run", "Run All", or both). */
  runningPlugins: Set<string>;
  /** True from the moment "Run All" is clicked until every operation it triggered has settled. */
  runAllActive: boolean;
  /** Trigger/timeout errors, most recent last. Distinct from a plugin's own FAILED result, which shows inline in its row. */
  runErrors: RunErrorEntry[];
  dismissError: (id: string) => void;
  refresh: () => Promise<void>;
  runOne: (pluginName: string) => Promise<void>;
  runAll: () => Promise<void>;
}

export function usePluginsStatus(): UsePluginsStatusResult {
  const queryClient = useQueryClient();
  const [runErrors, setRunErrors] = useState<RunErrorEntry[]>([]);
  // Plugin names triggered locally whose POST hasn't resolved yet — covers
  // the brief gap before the operations list has been refetched.
  const [pendingLocal, setPendingLocal] = useState<Set<string>>(new Set());
  // Operation names triggered by the most recent "Run All", still pending.
  const [pendingRunAllOps, setPendingRunAllOps] = useState<Set<string>>(new Set());
  const warnedRef = useRef<Set<string>>(new Set());
  const { token } = useOpenMFPContext();

  const addError = useCallback((message: string) => {
    setRunErrors((prev) => [
      ...prev,
      { id: `${Date.now()}-${Math.random().toString(36).slice(2)}`, message },
    ]);
  }, []);

  const dismissError = useCallback((id: string) => {
    setRunErrors((prev) => prev.filter((e) => e.id !== id));
  }, []);

  const pluginsQuery = useQuery({
    queryKey: queryKeys.plugins,
    queryFn: () => fetchPlugins(token),
  });

  const operationsQuery = useQuery({
    queryKey: queryKeys.operations,
    queryFn: () => fetchOperations(token),
    refetchInterval: (query) => {
      const ops = query.state.data ?? [];
      return ops.some((op) => !isTerminal(op)) ? POLL_INTERVAL_MS : false;
    },
  });

  const operations = operationsQuery.data ?? EMPTY_OPERATIONS;

  const runningPlugins = useMemo(() => {
    const running = new Set(
      operations.filter((op) => !isTerminal(op)).map((op) => op.plugin)
    );
    pendingLocal.forEach((p) => running.add(p));
    return running;
  }, [operations, pendingLocal]);

  // Surface a one-time notice for any operation that's been running too long.
  useEffect(() => {
    operations.filter(isStale).forEach((op) => {
      if (!warnedRef.current.has(op.name)) {
        warnedRef.current.add(op.name);
        addError(`"${op.plugin}" is taking longer than expected — check back shortly.`);
      }
    });
  }, [operations, addError]);

  // Clear "Run All" ops out of the pending set once they reach a terminal state.
  useEffect(() => {
    if (pendingRunAllOps.size === 0) return;
    setPendingRunAllOps((prev) => {
      const next = new Set(
        Array.from(prev).filter((name) => {
          const op = operations.find((o) => o.name === name);
          return op ? !isTerminal(op) : true;
        })
      );
      return next.size === prev.size ? prev : next;
    });
  }, [operations, pendingRunAllOps]);

  const runAllActive = pendingRunAllOps.size > 0;

  const refresh = useCallback(async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: queryKeys.plugins }),
      queryClient.invalidateQueries({ queryKey: queryKeys.operations }),
    ]);
  }, [queryClient]);

  const runOneMutation = useMutation({
    mutationFn: (pluginName: string) => apiRunPlugin(token, pluginName),
    onMutate: (pluginName: string) => {
      setPendingLocal((prev) => new Set(prev).add(pluginName));
    },
    onSuccess: (newOp) => {
      queryClient.setQueryData<OperationResource[]>(queryKeys.operations, (oldOps = []) =>
        mergeOperations(oldOps, [newOp])
      );
    },
    onError: (err, pluginName) => {
      addError(err instanceof Error ? err.message : `Failed to run "${pluginName}"`);
    },
    onSettled: (_data, _err, pluginName) => {
      setPendingLocal((prev) => {
        const next = new Set(prev);
        next.delete(pluginName);
        return next;
      });
    },
  });

  const runAllMutation = useMutation({
    mutationFn: () => apiRunAllPlugins(token),
    onSuccess: (ops) => {
      setPendingRunAllOps(new Set(ops.map((op) => op.name)));
      queryClient.setQueryData<OperationResource[]>(queryKeys.operations, (oldOps = []) =>
        mergeOperations(oldOps, ops)
      );
    },
    onError: (err) => {
      addError(err instanceof Error ? err.message : 'Failed to trigger plugin run');
    },
  });

  const runOne = useCallback(
    async (pluginName: string) => {
      await runOneMutation.mutateAsync(pluginName).catch(() => {});
    },
    [runOneMutation]
  );

  const runAll = useCallback(async () => {
    await runAllMutation.mutateAsync().catch(() => {});
  }, [runAllMutation]);

  return {
    plugins: pluginsQuery.data ?? [],
    operations,
    loading: pluginsQuery.isLoading || operationsQuery.isLoading,
    runningPlugins,
    runAllActive,
    runErrors,
    dismissError,
    refresh,
    runOne,
    runAll,
  };
}
