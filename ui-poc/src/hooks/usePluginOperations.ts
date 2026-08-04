import { useState, useCallback, useEffect, useRef } from 'react';
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

// ---------------------------------------------------------------------------
// usePluginsStatus — single source of truth for the plugins list and their
// run operations. Handles triggering a single plugin or all plugins, and
// only resolves/refreshes once the run(s) actually finished.
// ---------------------------------------------------------------------------

interface UsePluginsStatusResult {
  plugins: string[];
  operations: OperationResource[];
  loading: boolean;
  /** Plugin names currently in flight — via "Run", via "Run All", or both. */
  runningPlugins: Set<string>;
  /** True from the moment "Run All" is clicked until every triggered run has settled. */
  runAllActive: boolean;
  runError: string | null;
  refresh: () => Promise<void>;
  runOne: (pluginName: string) => Promise<void>;
  runAll: () => Promise<void>;
}

export function usePluginsStatus(): UsePluginsStatusResult {
  const [plugins, setPlugins] = useState<string[]>([]);
  const [operations, setOperations] = useState<OperationResource[]>([]);
  const [loading, setLoading] = useState(false);
  const [runningPlugins, setRunningPlugins] = useState<Set<string>>(new Set());
  const [runAllActive, setRunAllActive] = useState(false);
  const [runError, setRunError] = useState<string | null>(null);
  const cancelledRef = useRef(false);

  useEffect(() => {
    return () => {
      cancelledRef.current = true;
    };
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
        setRunError(err instanceof Error ? err.message : 'Failed to load plugin status');
      }
    } finally {
      if (!cancelledRef.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const markRunning = (name: string) =>
    setRunningPlugins((prev) => {
      const next = new Set(prev);
      next.add(name);
      return next;
    });

  const markDone = (name: string) =>
    setRunningPlugins((prev) => {
      const next = new Set(prev);
      next.delete(name);
      return next;
    });

  /**
   * Triggers a single operation and, once it settles, merges just that
   * operation's result into `operations` and clears it from `runningPlugins`
   * — independent of anything else that may still be running.
   */
  const settleOne = useCallback(async (op: OperationResource) => {
    const { operation, timedOut } = await pollSingle(op, cancelledRef);
    if (cancelledRef.current) return;

    if (timedOut) {
      setRunError(`"${op.plugin}" is taking longer than expected — check back shortly.`);
    } else {
      setOperations((prev) => mergeOperation(prev, operation));
    }
    markDone(op.plugin);
  }, []);

  const runOne = useCallback(
    async (pluginName: string) => {
      setRunError(null);
      markRunning(pluginName);
      try {
        const op = await apiRunPlugin(pluginName);
        await settleOne(op);
      } catch (err) {
        if (!cancelledRef.current) {
          setRunError(err instanceof Error ? err.message : `Failed to run "${pluginName}"`);
          markDone(pluginName);
        }
      }
    },
    [settleOne]
  );

  const runAll = useCallback(async () => {
    setRunError(null);
    setRunAllActive(true);
    try {
      const ops = await apiRunAllPlugins();
      ops.forEach((op) => markRunning(op.plugin));

      // Each operation is settled independently: a plugin that finishes in
      // 1s flips its row back to Run/status immediately, without waiting
      // for a slower one still running a minute later.
      await Promise.all(ops.map((op) => settleOne(op)));
    } catch (err) {
      if (!cancelledRef.current) {
        setRunError(err instanceof Error ? err.message : 'Failed to trigger plugin run');
      }
    } finally {
      if (!cancelledRef.current) setRunAllActive(false);
    }
  }, [settleOne]);

  return {
    plugins,
    operations,
    loading,
    runningPlugins,
    runAllActive,
    runError,
    refresh,
    runOne,
    runAll,
  };
}