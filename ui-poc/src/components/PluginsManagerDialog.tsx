import { useState } from 'react';
import { X, Play, RefreshCw } from 'lucide-react';
import { usePluginsStatus } from '../hooks/usePluginOperations';
import { OperationResource } from '../lib/catalogApi';
import { PluginStatusBadge } from './PluginStatusBadge';
import { PluginErrorLog } from './PluginErrorLog';
import { PluginRunHistory } from './PluginRunHistory';

interface PluginsManagerDialogProps {
  open: boolean;
  onClose: () => void;
  onRunsCompleted: () => void;
}

type Tab = 'status' | 'history';

/**
 * Modal dialog for managing plugin ingestion.
 *
 * - "Status" tab: one row per plugin with its latest run state
 * - "Full History" tab: complete execution history
 * - "Run All Plugins" button at the top
 */
export function PluginsManagerDialog({ open, onClose, onRunsCompleted }: PluginsManagerDialogProps) {
  const [tab, setTab] = useState<Tab>('status');

  const { plugins, operations, loading, runningPlugins, runAllActive, runError, refresh, runOne, runAll } =
    usePluginsStatus();

  const anyRunning = runningPlugins.size > 0 || runAllActive;

  const handleRunAll = async () => {
    await runAll(); // resolves only once every triggered operation is terminal (or timed out)
    onRunsCompleted();
  };

  const handleRunSingle = async (pluginName: string) => {
    await runOne(pluginName);
    onRunsCompleted();
  };

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={onClose}
      role="dialog"
      aria-modal="true"
    >
      <div
        className="flex max-h-[80vh] w-full max-w-3xl flex-col rounded-lg bg-white shadow-xl dark:bg-background-dark-paper"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex shrink-0 items-center justify-between border-b border-gray-200 px-6 py-4 dark:border-gray-700">
          <div>
            <h2 className="text-lg font-semibold text-foreground dark:text-foreground-dark-default">
              Plugins & Ingestion
            </h2>
            <p className="text-xs text-foreground-secondary dark:text-foreground-dark-secondary">
              Run individual plugins or all at once, and inspect their latest status.
            </p>
          </div>
          <button
            onClick={onClose}
            className="rounded-md p-1.5 text-gray-500 hover:bg-gray-100 hover:text-gray-700 dark:text-gray-400 dark:hover:bg-gray-800"
            aria-label="Close"
          >
            <X size={18} />
          </button>
        </div>

        {/* Toolbar: tabs + Run All */}
        <div className="flex shrink-0 items-center justify-between border-b border-gray-200 px-6 py-3 dark:border-gray-700">
          <div className="flex gap-1">
            <TabButton active={tab === 'status'} onClick={() => setTab('status')}>
              Status
            </TabButton>
            <TabButton active={tab === 'history'} onClick={() => setTab('history')}>
              Full History
            </TabButton>
          </div>
          <button
            onClick={handleRunAll}
            disabled={anyRunning}
            className="inline-flex items-center gap-2 rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
          >
            <RefreshCw size={14} className={runAllActive ? 'animate-spin' : ''} />
            {runAllActive ? 'Running All…' : 'Run All Plugins'}
          </button>
        </div>

        {/* Body */}
        <div className="min-h-0 flex-1 overflow-y-auto px-6 py-4">
          {runError && (
            <div className="mb-4 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-600 dark:border-red-800 dark:bg-red-950 dark:text-red-400">
              {runError}
            </div>
          )}

          {tab === 'status' ? (
            <StatusTab
              plugins={plugins}
              operations={operations}
              loading={loading}
              runningPlugins={runningPlugins}
              anyRunning={anyRunning}
              onRun={handleRunSingle}
            />
          ) : (
            <PluginRunHistory operations={operations} loading={loading} onRefresh={refresh} />
          )}
        </div>
      </div>
    </div>
  );
}

function TabButton({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      onClick={onClick}
      className={`rounded-md px-3 py-1.5 text-sm font-medium transition-colors ${
        active
          ? 'bg-primary/10 text-primary dark:bg-primary/20'
          : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'
      }`}
    >
      {children}
    </button>
  );
}

function StatusTab({
  plugins,
  operations,
  loading,
  runningPlugins,
  anyRunning,
  onRun,
}: {
  plugins: string[];
  operations: OperationResource[];
  loading: boolean;
  runningPlugins: Set<string>;
  anyRunning: boolean;
  onRun: (plugin: string) => void;
}) {
  const latestByPlugin = latestOperationPerPlugin(operations);

  if (plugins.length === 0 && !loading) {
    return (
      <p className="py-8 text-center text-sm text-gray-500 dark:text-gray-400">
        No plugins registered.
      </p>
    );
  }

  return (
    <table className="w-full text-left text-sm">
      <thead>
        <tr className="border-b border-gray-200 text-xs uppercase tracking-wide text-gray-500 dark:border-gray-700 dark:text-gray-400">
          <th className="py-2 pr-4 font-medium">Plugin</th>
          <th className="py-2 pr-4 font-medium">Last run</th>
          <th className="py-2 pr-4 font-medium">Status</th>
          <th className="py-2 pr-4 font-medium">Result</th>
          <th className="py-2 text-right font-medium">Action</th>
        </tr>
      </thead>
      <tbody>
        {plugins.map((plugin) => {
          const op = latestByPlugin.get(plugin);
          // Running via its own button, or swept up in "Run All" — either way
          // the previous status/result is stale and shouldn't be shown next
          // to a spinning Run button. Cleared per-plugin as soon as *that*
          // plugin's own run settles, independent of any others still going.
          const running = runningPlugins.has(plugin);

          return (
            <tr key={plugin} className="border-b border-gray-100 last:border-0 dark:border-gray-800">
              <td className="py-3 pr-4 font-medium text-gray-900 dark:text-gray-100">{plugin}</td>
              <td className="py-3 pr-4 text-gray-500 dark:text-gray-400">
                {op ? formatRelativeTime(op.createdAt) : 'Never'}
              </td>
              <td className="py-3 pr-4">
                {running ? (
                  <span className="text-xs text-gray-400">—</span>
                ) : op ? (
                  <PluginStatusBadge state={op.state} />
                ) : (
                  <span className="text-xs text-gray-400">Not run yet</span>
                )}
              </td>
              <td className="py-3 pr-4 text-gray-500 dark:text-gray-400">
                {running ? (
                  <span className="text-xs text-gray-400">—</span>
                ) : op && op.state === 'SUCCEEDED' ? (
                  `${op.nodesUpserted} node(s), ${op.relationsUpserted} relation(s)`
                ) : op && op.state === 'FAILED' && op.error ? (
                  <PluginErrorLog error={op.error} />
                ) : (
                  <span className="text-xs text-gray-400">—</span>
                )}
              </td>
              <td className="py-3 text-right">
                <button
                  onClick={() => onRun(plugin)}
                  disabled={anyRunning}
                  className="inline-flex items-center gap-1.5 rounded-md bg-primary px-2.5 py-1.5 text-xs font-medium text-white transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
                  title={`Run ${plugin} plugin`}
                >
                  {running ? <RefreshCw size={12} className="animate-spin" /> : <Play size={12} />}
                  {running ? 'Running…' : 'Run'}
                </button>
              </td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}

/**
 * Returns a map of plugin name → most recent operation. Sorts defensively
 * by createdAt rather than assuming API ordering, since the UI's
 * correctness would otherwise silently depend on an unenforced API contract.
 */
function latestOperationPerPlugin(operations: OperationResource[]): Map<string, OperationResource> {
  const sorted = [...operations].sort(
    (a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime()
  );
  const latest = new Map<string, OperationResource>();
  for (const op of sorted) {
    if (!latest.has(op.plugin)) {
      latest.set(op.plugin, op);
    }
  }
  return latest;
}

function formatRelativeTime(iso: string): string {
  const then = new Date(iso).getTime();
  const diffMs = Date.now() - then;
  const seconds = Math.max(0, Math.floor(diffMs / 1000));

  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes} min ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}