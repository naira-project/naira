import { useState, useCallback, useEffect, useRef } from 'react';
import {
  OperationResource,
  runAllPlugins as apiRunAllPlugins,
  runPlugin as apiRunPlugin,
  fetchOperation,
  fetchOperations,
} from '../lib/catalogApi';

const POLL_INTERVAL_MS = 2000;

// ---------------------------------------------------------------------------
// usePluginOperations — fetch operation history
// ---------------------------------------------------------------------------

interface UsePluginOperationsResult {
  operations: OperationResource[];
  loading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
}

export function usePluginOperations(): UsePluginOperationsResult {
  const [operations, setOperations] = useState<OperationResource[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const ops = await fetchOperations();
      setOperations(ops);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch operations');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  return { operations, loading, error, refresh };
}

// ---------------------------------------------------------------------------
// usePluginRun — trigger a single plugin and poll until done
// ---------------------------------------------------------------------------

interface UsePluginRunResult {
  running: boolean;
  operation: OperationResource | null;
  error: string | null;
  trigger: () => Promise<void>;
}

export function usePluginRun(pluginName: string): UsePluginRunResult {
  const [running, setRunning] = useState(false);
  const [operation, setOperation] = useState<OperationResource | null>(null);
  const [error, setError] = useState<string | null>(null);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const clearPoll = useCallback(() => {
    if (intervalRef.current !== null) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    }
  }, []);

  const trigger = useCallback(async () => {
    setRunning(true);
    setError(null);
    setOperation(null);

    try {
      const op = await apiRunPlugin(pluginName);
      setOperation(op);

      // Poll until the operation is done
      intervalRef.current = setInterval(async () => {
        try {
          const updated = await fetchOperation(op.name);
          setOperation(updated);
          if (updated.state === 'SUCCEEDED' || updated.state === 'FAILED') {
            clearPoll();
            setRunning(false);
          }
        } catch {
          clearPoll();
          setRunning(false);
          setError('Failed to poll operation status');
        }
      }, POLL_INTERVAL_MS);
    } catch (err) {
      setRunning(false);
      setError(err instanceof Error ? err.message : 'Failed to trigger plugin run');
    }
  }, [pluginName, clearPoll]);

  // Cleanup polling on unmount
  useEffect(() => {
    return () => clearPoll();
  }, [clearPoll]);

  return { running, operation, error, trigger };
}

// ---------------------------------------------------------------------------
// useRunPluginAction — trigger a single plugin by name and poll until done.
// Unlike usePluginRun, the plugin name is supplied at trigger time, so this
// can be used for a dynamic list of plugins (e.g. a table row per plugin).
// ---------------------------------------------------------------------------

interface UseRunPluginActionResult {
  runningPlugin: string | null;
  error: string | null;
  trigger: (pluginName: string) => Promise<void>;
}

export function useRunPluginAction(onComplete?: () => void): UseRunPluginActionResult {
  const [runningPlugin, setRunningPlugin] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const cancelledRef = useRef(false);

  const trigger = useCallback(
    async (pluginName: string) => {
      setRunningPlugin(pluginName);
      setError(null);

      try {
        const op = await apiRunPlugin(pluginName);

        // Poll until the operation reaches a terminal state.
        while (!cancelledRef.current) {
          const updated = await fetchOperation(op.name);
          if (updated.state === 'SUCCEEDED' || updated.state === 'FAILED') {
            break;
          }
          await new Promise((resolve) => setTimeout(resolve, POLL_INTERVAL_MS));
        }

        onComplete?.();
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to trigger plugin run');
      } finally {
        if (!cancelledRef.current) {
          setRunningPlugin(null);
        }
      }
    },
    [onComplete]
  );

  // Mark cancelled on unmount so in-flight polling does not update state.
  useEffect(() => {
    return () => {
      cancelledRef.current = true;
    };
  }, []);

  return { runningPlugin, error, trigger };
}

// ---------------------------------------------------------------------------
// useAllPluginsRun — trigger all plugins and poll until all are done
// ---------------------------------------------------------------------------

interface UseAllPluginsRunResult {
  running: boolean;
  operations: OperationResource[];
  error: string | null;
  trigger: () => Promise<void>;
}

export function useAllPluginsRun(): UseAllPluginsRunResult {
  const [running, setRunning] = useState(false);
  const [operations, setOperations] = useState<OperationResource[]>([]);
  const [error, setError] = useState<string | null>(null);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const clearPoll = useCallback(() => {
    if (intervalRef.current !== null) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    }
  }, []);

  const trigger = useCallback(async () => {
    setRunning(true);
    setError(null);
    setOperations([]);

    try {
      const ops = await apiRunAllPlugins();
      setOperations(ops);

      if (ops.length === 0) {
        setRunning(false);
        return;
      }

      // Poll all operations until every one is done
      intervalRef.current = setInterval(async () => {
        try {
          const updated = await Promise.all(
            ops.map((op) => fetchOperation(op.name))
          );
          setOperations(updated);

          const allDone = updated.every(
            (op) => op.state === 'SUCCEEDED' || op.state === 'FAILED'
          );
          if (allDone) {
            clearPoll();
            setRunning(false);
          }
        } catch {
          clearPoll();
          setRunning(false);
          setError('Failed to poll operation status');
        }
      }, POLL_INTERVAL_MS);
    } catch (err) {
      setRunning(false);
      setError(err instanceof Error ? err.message : 'Failed to trigger plugin run');
    }
  }, [clearPoll]);

  // Cleanup polling on unmount
  useEffect(() => {
    return () => clearPoll();
  }, [clearPoll]);

  return { running, operations, error, trigger };
}