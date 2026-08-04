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
 * Polls the given operations until every one reaches a terminal state,
 * or until POLL_MAX_ATTEMPTS is exceeded. Never hangs forever.
 *
 * `cancelledRef` lets the caller stop updating state after unmount.
 */
async function pollUntilDone(
  ops: OperationResource[],
  cancelledRef: React.RefObject<boolean>
): Promise<{ operations: OperationResource[]; timedOut: boolean }> {
  let current = ops;

  for (let attempt = 0; attempt < POLL_MAX_ATTEMPTS; attempt++) {
    if (current.every(isTerminal)) {
      return { operations: current, timedOut: false };
    }
    if (cancelledRef.current) {
      return { operations: current, timedOut: false };
    }

    await new Promise((resolve) => setTimeout(resolve, POLL_INTERVAL_MS));
    if (cancelledRef.current) {
      return { operations: current, timedOut: false };
    }

    current = await Promise.all(current.map((op) => fetchOperation(op.name)));
  }

  return { operations: current, timedOut: !current.every(isTerminal) };
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
  /** Name of the single plugin currently running, or 'all', or null. */
  runningKey: string | 'all' | null;
  runError: string | null;
  refresh: () => Promise<void>;
  runOne: (pluginName: string) => Promise<void>;
  runAll: () => Promise<void>;
}

export function usePluginsStatus(): UsePluginsStatusResult {
  const [plugins, setPlugins] = useState<string[]>([]);
  const [operations, setOperations] = useState<OperationResource[]>([]);
  const [loading, setLoading] = useState(false);
  const [runningKey, setRunningKey] = useState<string | 'all' | null>(null);
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

  const runOne = useCallback(
    async (pluginName: string) => {
      setRunningKey(pluginName);
      setRunError(null);
      try {
        const op = await apiRunPlugin(pluginName);
        const { timedOut } = await pollUntilDone([op], cancelledRef);
        if (timedOut) {
          setRunError(`"${pluginName}" is taking longer than expected — check back shortly.`);
        }
        if (!cancelledRef.current) await refresh();
      } catch (err) {
        if (!cancelledRef.current) {
          setRunError(err instanceof Error ? err.message : `Failed to run "${pluginName}"`);
        }
      } finally {
        if (!cancelledRef.current) setRunningKey(null);
      }
    },
    [refresh]
  );

  const runAll = useCallback(async () => {
    setRunningKey('all');
    setRunError(null);
    try {
      const ops = await apiRunAllPlugins();
      if (ops.length > 0) {
        const { timedOut } = await pollUntilDone(ops, cancelledRef);
        if (timedOut) {
          setRunError('Some plugins are taking longer than expected — check back shortly.');
        }
      }
      if (!cancelledRef.current) await refresh();
    } catch (err) {
      if (!cancelledRef.current) {
        setRunError(err instanceof Error ? err.message : 'Failed to trigger plugin run');
      }
    } finally {
      if (!cancelledRef.current) setRunningKey(null);
    }
  }, [refresh]);

  return { plugins, operations, loading, runningKey, runError, refresh, runOne, runAll };
}