import { useMutation, useMutationState, useQuery, useQueryClient } from '@tanstack/react-query';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  runAllPlugins as apiRunAllPlugins,
  runPlugin as apiRunPlugin,
  fetchOperations,
  fetchPlugins,
  type OperationResource,
  type PluginResource,
} from '../lib/catalogApi';
import { queryKeys } from '../lib/queryKeys';
import { useOpenMFPContext } from './useOpenMFPContext';

const ACTIVE_POLL_MS = 2000;
const IDLE_POLL_MS = 20_000;
const STALE_AFTER_MS = 2 * 60 * 1000;
const EMPTY_OPERATIONS: OperationResource[] = [];
const RUN_MUTATION_KEY = ['plugins', 'run'] as const;

/** Plugin names to run, or `null` for all registered plugins. */
type RunTarget = string[] | null;

interface RunResult {
  operations: OperationResource[];
  errors: string[];
}

function isStale(op: OperationResource) {
  if (op.done) return false;

  const startTime = new Date(op.metadata.startTime).getTime();
  const createdTime = new Date(op.metadata.createdAt).getTime();
  const referenceTime = Number.isFinite(startTime) && startTime > 0 ? startTime : createdTime;

  return Number.isFinite(referenceTime) && Date.now() - referenceTime > STALE_AFTER_MS;
}

function mergeOperations(
  current: OperationResource[] = [],
  incoming: OperationResource[],
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
 * Polls plugin run operations and invalidates catalog data whenever a run
 * finishes successfully, regardless of who started it (UI or scheduler).
 *
 * Polls every 2s while any operation is unfinished and every 20s otherwise,
 * and refetches on window focus. Each poll returns the full list, so a missed
 * response is repaired by the next one.
 *
 * Can be mounted on its own to keep catalog data fresh outside the Plugins page.
 */
export function useOperationsSync() {
  const queryClient = useQueryClient();
  const { token } = useOpenMFPContext();

  const query = useQuery({
    queryKey: queryKeys.operations,
    queryFn: () => fetchOperations(token),
    refetchOnWindowFocus: true,
    refetchInterval: (currentQuery) =>
      (currentQuery.state.data ?? []).some((op) => !op.done) ? ACTIVE_POLL_MS : IDLE_POLL_MS,
  });

  const initializedRef = useRef(false);
  const finishedSeenRef = useRef<Set<string>>(new Set());
  const runningSeenRef = useRef<Set<string>>(new Set());

  useEffect(() => {
    const operations = query.data;
    if (!operations) return;

    const finishedSeen = finishedSeenRef.current;
    const present = new Set(operations.map((op) => op.name));
    const running = new Set<string>();
    let catalogChanged = false;

    for (const op of operations) {
      if (!op.done) {
        running.add(op.name);
      } else if (!finishedSeen.has(op.name)) {
        finishedSeen.add(op.name);
        // The first load only records history; it must not trigger a refetch.
        if (initializedRef.current && !op.error) catalogChanged = true;
      }
    }

    // An operation seen running that is gone from the list finished unobserved.
    if (initializedRef.current) {
      for (const name of runningSeenRef.current) {
        if (!present.has(name)) catalogChanged = true;
      }
    }

    runningSeenRef.current = running;
    initializedRef.current = true;

    if (catalogChanged) {
      void queryClient.invalidateQueries({ queryKey: queryKeys.catalog });
    }
  }, [query.data, queryClient]);

  return query;
}

interface UsePluginsStatusResult {
  plugins: PluginResource[];
  operations: OperationResource[];
  loading: boolean;
  /** Plugins with an unfinished operation or a run request in flight. */
  runningPlugins: Set<string>;
  /** True while any plugin is running. */
  runAllActive: boolean;
  runErrors: RunErrorEntry[];
  dismissError: (id: string) => void;
  refresh: () => Promise<void>;
  runOne: (pluginName: string) => Promise<void>;
  runAll: () => Promise<void>;
  runSubset: (pluginNames: string[]) => Promise<void>;
}

/**
 * Plugins list plus their run state. Running state is derived from the
 * operations returned by the backend; the only local state is the set of run
 * requests awaiting a response, tracked by the mutation itself.
 */
export function usePluginsStatus(): UsePluginsStatusResult {
  const queryClient = useQueryClient();
  const { token } = useOpenMFPContext();
  const [runErrors, setRunErrors] = useState<RunErrorEntry[]>([]);
  const warnedRef = useRef<Set<string>>(new Set());

  const addError = useCallback((message: string) => {
    setRunErrors((previous) => [
      ...previous,
      { id: `${Date.now()}-${Math.random().toString(36).slice(2)}`, message },
    ]);
  }, []);

  const dismissError = useCallback((id: string) => {
    setRunErrors((previous) => previous.filter((error) => error.id !== id));
  }, []);

  const pluginsQuery = useQuery({
    queryKey: queryKeys.plugins,
    queryFn: () => fetchPlugins(token),
  });
  const operationsQuery = useOperationsSync();

  const plugins = pluginsQuery.data ?? [];
  const operations = operationsQuery.data ?? EMPTY_OPERATIONS;

  const runMutation = useMutation<RunResult, Error, RunTarget>({
    mutationKey: RUN_MUTATION_KEY,
    mutationFn: async (target) => {
      if (target === null) {
        return { operations: await apiRunAllPlugins(token), errors: [] };
      }
      const settled = await Promise.allSettled(target.map((name) => apiRunPlugin(token, name)));
      const result: RunResult = { operations: [], errors: [] };
      settled.forEach((outcome, index) => {
        if (outcome.status === 'fulfilled') {
          result.operations.push(outcome.value);
        } else {
          result.errors.push(
            outcome.reason instanceof Error
              ? outcome.reason.message
              : `Failed to run "${target[index]}"`,
          );
        }
      });
      return result;
    },
    onSuccess: ({ operations: started, errors }) => {
      // Show the new operations immediately instead of waiting for the next poll.
      queryClient.setQueryData<OperationResource[]>(queryKeys.operations, (old) =>
        mergeOperations(old, started),
      );
      // Each cache write restarts the refetchInterval timer, so rapid runs would
      // starve polling. Refetch now to keep the list fresh.
      void queryClient.invalidateQueries({ queryKey: queryKeys.operations });
      errors.forEach(addError);
    },
    onError: (error) => {
      addError(error.message || 'Failed to trigger plugin run');
    },
  });

  // Several run requests can be in flight at once, so read all pending ones.
  const pendingTargets = useMutationState({
    filters: { mutationKey: RUN_MUTATION_KEY, status: 'pending' },
    select: (mutation) => mutation.state.variables as RunTarget,
  });

  const runningPlugins = useMemo(() => {
    const running = new Set(operations.filter((op) => !op.done).map((op) => op.metadata.plugin));
    for (const target of pendingTargets) {
      for (const name of target ?? plugins.map((plugin) => plugin.name)) {
        running.add(name);
      }
    }
    return running;
  }, [operations, pendingTargets, plugins]);

  // Warn once per operation that exceeds the stale threshold.
  useEffect(() => {
    operations.filter(isStale).forEach((op) => {
      if (!warnedRef.current.has(op.name)) {
        warnedRef.current.add(op.name);
        addError(`"${op.metadata.plugin}" is taking longer than expected — check back shortly.`);
      }
    });
  }, [operations, addError]);

  const refresh = useCallback(async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: queryKeys.plugins }),
      queryClient.invalidateQueries({ queryKey: queryKeys.operations }),
    ]);
  }, [queryClient]);

  const { mutateAsync } = runMutation;
  const run = useCallback(
    async (target: RunTarget) => {
      await mutateAsync(target).catch(() => {});
    },
    [mutateAsync],
  );

  const runOne = useCallback((pluginName: string) => run([pluginName]), [run]);
  const runAll = useCallback(() => run(null), [run]);
  const runSubset = useCallback((pluginNames: string[]) => run(pluginNames), [run]);

  return {
    plugins,
    operations,
    loading: pluginsQuery.isLoading || operationsQuery.isLoading,
    runningPlugins,
    runAllActive: runningPlugins.size > 0,
    runErrors,
    dismissError,
    refresh,
    runOne,
    runAll,
    runSubset,
  };
}
