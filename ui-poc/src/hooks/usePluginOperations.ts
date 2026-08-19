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

/**
 * Single source of truth for the plugins list and their run operations.
 *
 * Each plugin's running state is derived directly from the operations list,
 * allowing single plugin runs and batch runs ("Run All") to operate independently.
 * Polling for operations automatically activates every 2 seconds via React Query
 * whenever non-terminal operations exist.
 */
interface UsePluginsStatusResult {
  plugins: string[];
  operations: OperationResource[];
  loading: boolean;
  /** Set of plugin names currently executing or awaiting response. */
  runningPlugins: Set<string>;
  /** Indicates whether a "Run All" execution is currently in progress. */
  runAllActive: boolean;
  /** List of execution or timeout errors. */
  runErrors: RunErrorEntry[];
  dismissError: (id: string) => void;
  refresh: () => Promise<void>;
  runOne: (pluginName: string) => Promise<void>;
  runAll: () => Promise<void>;
  runSubset: (pluginNames: string[]) => Promise<void>;
}

export function usePluginsStatus(): UsePluginsStatusResult {
  const queryClient = useQueryClient();
  const [runErrors, setRunErrors] = useState<RunErrorEntry[]>([]);
  
  // Tracks locally triggered plugins while waiting for the POST mutation response
  const [pendingLocal, setPendingLocal] = useState<Set<string>>(new Set());
  
  // Tracks operation names triggered by "Run All" until they reach a terminal state
  const [pendingRunAllOps, setPendingRunAllOps] = useState<Set<string>>(new Set());

  // Indicates whether a "Run Subset" execution is currently in progress
  const [subsetActive, setSubsetActive] = useState(false);

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

  // Emit a warning notice if an operation exceeds the stale threshold
  useEffect(() => {
    operations.filter(isStale).forEach((op) => {
      if (!warnedRef.current.has(op.name)) {
        warnedRef.current.add(op.name);
        addError(`"${op.plugin}" is taking longer than expected — check back shortly.`);
      }
    });
  }, [operations, addError]);

  // Remove settled operations from the "Run All" tracking set
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

  const runAllActive = pendingRunAllOps.size > 0 || runAllMutation.isPending || subsetActive;

  /**
   * Like `runAll`, but scoped to a specific set of plugins — used by viewpoints
   * (e.g. Model, Software Catalog) that only want to trigger the plugins they show,
   * rather than every plugin the backend knows about.
   */
  const runSubset = useCallback(
    async (pluginNames: string[]) => {
      setPendingLocal((prev) => {
        const next = new Set(prev);
        pluginNames.forEach((name) => next.add(name));
        return next;
      });
      setSubsetActive(true);
      try {
        const results = await Promise.allSettled(
          pluginNames.map(async (name) => {
            const op = await apiRunPlugin(token, name);
            queryClient.setQueryData<OperationResource[]>(queryKeys.operations, (oldOps = []) =>
              mergeOperations(oldOps, [op])
            );
          })
        );
        results.forEach((result, i) => {
          if (result.status === 'rejected') {
            addError(
              result.reason instanceof Error
                ? result.reason.message
                : `Failed to run "${pluginNames[i]}"`
            );
          }
        });
      } finally {
        setPendingLocal((prev) => {
          const next = new Set(prev);
          pluginNames.forEach((name) => next.delete(name));
          return next;
        });
        setSubsetActive(false);
      }
    },
    [token, queryClient, addError]
  );

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
    runSubset,
  };
}