import { useState, useCallback, useEffect, useRef, useMemo } from 'react';
import {
  OperationResource,
  runAllPlugins as apiRunAllPlugins,
  runPlugin as apiRunPlugin,
  fetchOperation,
  fetchOperations,
  fetchPlugins,
} from '../lib/catalogApi';

const POLL_INTERVAL_MS = 2000;
const POLL_MAX_ATTEMPTS = 60; // 60 * 2s = 2 minutes

const TERMINAL_STATES: OperationResource['state'][] = ['SUCCEEDED', 'FAILED'];

function isTerminal(op: OperationResource) {
  return TERMINAL_STATES.includes(op.state);
}

/**
 * Polls a single operation until it reaches a terminal state, or until
 * POLL_MAX_ATTEMPTS is exceeded. Never hangs forever.
 *
 * Each operation is polled independently — callers running several of these
 * concurrently (e.g. "Run All") get each result as soon as *that* operation
 * finishes, instead of waiting for the slowest one.
 *
 * `cancelledRef` lets the caller stop updating state after unmount.
 */
async function pollSingle(
  op: OperationResource,
  cancelledRef: React.MutableRefObject<boolean>
): Promise<{ operation: OperationResource; timedOut: boolean }> {
  let current = op;

  for (let attempt = 0; attempt < POLL_MAX_ATTEMPTS; attempt++) {
    if (isTerminal(current) || cancelledRef.current) {
      return { operation: current, timedOut: false };
    }

    await new Promise((resolve) => setTimeout(resolve, POLL_INTERVAL_MS));
    if (cancelledRef.current) {
      return { operation: current, timedOut: false };
    }

    current = await fetchOperation(current.name);
  }

  return { operation: current, timedOut: !isTerminal(current) };
}

/**
 * Replaces the operation with the same name if present, otherwise prepends
 * it, keeping the most-recent-first ordering used elsewhere.
 */
function mergeOperation(list: OperationResource[], updated: OperationResource): OperationResource[] {
  const idx = list.findIndex((o) => o.name === updated.name);
  if (idx === -1) return [updated, ...list];
  const next = [...list];
  next[idx] = updated;
  return next;
}

export interface RunErrorEntry {
  id: string;
  message: string;
}

// ---------------------------------------------------------------------------
// usePluginsStatus — single source of truth for the plugins list and their
// run operations. "Run" (per plugin) and "Run All" are fully independent:
// each plugin tracks its own in-flight count, so triggering one plugin never
// disables the button for another, and "Run All" never waits on — or is
// blocked by — anything triggered individually.
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
  const [plugins, setPlugins] = useState<string[]>([]);
  const [operations, setOperations] = useState<OperationResource[]>([]);
  const [loading, setLoading] = useState(false);
  // Refcounted per plugin: a plugin can end up with two in-flight operations
  // at once (e.g. clicked individually, then swept into a "Run All") and
  // should only look "done" once *all* of its in-flight runs have settled.
  const [runningCounts, setRunningCounts] = useState<Map<string, number>>(new Map());
  const [runAllActive, setRunAllActive] = useState(false);
  const [runErrors, setRunErrors] = useState<RunErrorEntry[]>([]);
  const cancelledRef = useRef(false);

  const runningPlugins = useMemo(() => new Set(runningCounts.keys()), [runningCounts]);

  useEffect(() => {
    return () => {
      cancelledRef.current = true;
    };
  }, []);

  const addError = useCallback((message: string) => {
    setRunErrors((prev) => [
      ...prev,
      { id: `${Date.now()}-${Math.random().toString(36).slice(2)}`, message },
    ]);
  }, []);

  const dismissError = useCallback((id: string) => {
    setRunErrors((prev) => prev.filter((e) => e.id !== id));
  }, []);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const [pluginList, ops] = await Promise.all([fetchPlugins(), fetchOperations()]);
      if (cancelledRef.current) return;
      setPlugins(pluginList);
      setOperations(ops);
    } catch (err) {
      if (!cancelledRef.current) {
        addError(err instanceof Error ? err.message : 'Failed to load plugin status');
      }
    } finally {
      if (!cancelledRef.current) setLoading(false);
    }
  }, [addError]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const incRunning = (name: string) =>
    setRunningCounts((prev) => {
      const next = new Map(prev);
      next.set(name, (next.get(name) ?? 0) + 1);
      return next;
    });

  const decRunning = (name: string) =>
    setRunningCounts((prev) => {
      const count = prev.get(name) ?? 0;
      if (count <= 1) {
        if (!prev.has(name)) return prev;
        const next = new Map(prev);
        next.delete(name);
        return next;
      }
      const next = new Map(prev);
      next.set(name, count - 1);
      return next;
    });

  /**
   * Triggers a single operation and, once it settles, merges just that
   * operation's result into `operations` and decrements that plugin's
   * running count — independent of anything else that may still be
   * running, for that plugin or any other.
   */
  const settleOne = useCallback(
    async (op: OperationResource) => {
      const { operation, timedOut } = await pollSingle(op, cancelledRef);
      if (cancelledRef.current) return;

      if (timedOut) {
        addError(`"${op.plugin}" is taking longer than expected — check back shortly.`);
      } else {
        setOperations((prev) => mergeOperation(prev, operation));
      }
      decRunning(op.plugin);
    },
    [addError]
  );

  const runOne = useCallback(
    async (pluginName: string) => {
      incRunning(pluginName);
      try {
        const op = await apiRunPlugin(pluginName);
        await settleOne(op);
      } catch (err) {
        if (!cancelledRef.current) {
          addError(err instanceof Error ? err.message : `Failed to run "${pluginName}"`);
          decRunning(pluginName);
        }
      }
    },
    [settleOne, addError]
  );

  const runAll = useCallback(async () => {
    setRunAllActive(true);
    try {
      const ops = await apiRunAllPlugins();
      ops.forEach((op) => incRunning(op.plugin));

      // Each operation is settled independently: a plugin that finishes in
      // 1s flips its row back to Run/status immediately, without waiting
      // for a slower one still running a minute later. "Run All" itself
      // isn't blocked by, and doesn't block, anything triggered per-plugin.
      await Promise.all(ops.map((op) => settleOne(op)));
    } catch (err) {
      if (!cancelledRef.current) {
        addError(err instanceof Error ? err.message : 'Failed to trigger plugin run');
      }
    } finally {
      if (!cancelledRef.current) setRunAllActive(false);
    }
  }, [settleOne, addError]);

  return {
    plugins,
    operations,
    loading,
    runningPlugins,
    runAllActive,
    runErrors,
    dismissError,
    refresh,
    runOne,
    runAll,
  };
}