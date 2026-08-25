import { useEffect, useMemo, useState } from 'react';
import { useSearchParams } from 'react-router';
import { X, Play, RefreshCw, AlertCircle } from 'lucide-react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { usePluginsStatus } from '../hooks/usePluginOperations';
import {
  OperationResource,
  ScheduleResource,
  StatusErrorResource,
  fetchSchedules,
  updateSchedule,
} from '../lib/catalogApi';
import { PluginStatusBadge } from '../components/PluginStatusBadge';
import { PluginErrorModal } from '../components/PluginErrorModal';
import { formatDuration, formatRelativeTime, latestOperationPerPlugin } from '../lib/utils';
import { useOpenMFPContext } from '../hooks/useOpenMFPContext';
import { queryKeys } from '../lib/queryKeys';

/**
 * Dedicated page for managing plugin ingestion.
 *
 * Shows one row per plugin with its latest run state and a "Run All Plugins"
 * button at the top. Every "Run" button — including "Run All" — is independent:
 * triggering one plugin never disables another plugin's button, and "Run All"
 * neither blocks nor is blocked by anything triggered individually.
 *
 * Supports an optional `?only=plugin1,plugin2` query param to scope the page
 * to a subset of plugins, used by empty-state links from catalog viewpoints.
 */
export default function PluginsPage() {
  const [searchParams] = useSearchParams();
  const only = searchParams.get('only');
  const allowedPlugins = only ? only.split(',').map((s) => s.trim()).filter(Boolean) : undefined;

  const {
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
    runSubset,
  } = usePluginsStatus();
  const { token } = useOpenMFPContext();
  const queryClient = useQueryClient();
  const schedulesQuery = useQuery({
    queryKey: queryKeys.schedules,
    queryFn: () => fetchSchedules(token),
  });
  const scheduleMutation = useMutation({
    mutationFn: ({ plugin, expression, enabled }: { plugin: string; expression: string; enabled: boolean }) =>
      updateSchedule(token, plugin, { expression: expression || undefined, enabled }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.schedules }),
  });
  const schedules = useMemo(
    () => new Map((schedulesQuery.data ?? []).map((schedule) => [schedule.plugin, schedule])),
    [schedulesQuery.data]
  );

  const visiblePlugins = allowedPlugins
    ? plugins.filter((p) => allowedPlugins.includes(p))
    : plugins;

  const [selectedError, setSelectedError] = useState<{ plugin: string; error: StatusErrorResource } | null>(null);

  const handleRunVisible = async () => {
    if (allowedPlugins) {
      await runSubset(visiblePlugins);
    } else {
      await runAll();
    }
  };

  const handleRunSingle = async (pluginName: string) => {
    await runOne(pluginName);
  };

  return (
    <div className="flex h-screen overflow-hidden bg-background dark:bg-background-dark-default">
      <div className="flex flex-1 flex-col overflow-hidden">
        {/* Top bar */}
        <header className="flex shrink-0 items-center gap-3 border-b border-gray-200 bg-white px-6 py-3 dark:border-gray-700 dark:bg-background-dark-paper">
          <div>
            <h1 className="text-lg font-semibold text-foreground dark:text-foreground-dark-default">
              Plugins & Ingestion
            </h1>
            <p className="text-xs text-foreground-secondary dark:text-foreground-dark-secondary">
              Run individual plugins or all at once, and inspect their latest status.
            </p>
          </div>

          <div className="flex-1" />

          <button
            onClick={refresh}
            disabled={loading}
            className="inline-flex items-center justify-center rounded-md p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 disabled:cursor-not-allowed disabled:opacity-50 dark:text-gray-400 dark:hover:bg-gray-800"
            title="Refresh status"
            aria-label="Refresh status"
          >
            <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
          </button>
          <button
            onClick={handleRunVisible}
            disabled={runAllActive}
            className="inline-flex items-center gap-2 rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
          >
            <RefreshCw size={14} className={runAllActive ? 'animate-spin' : ''} />
            {runAllActive ? 'Running…' : allowedPlugins ? 'Run Shown Plugins' : 'Run All Plugins'}
          </button>
        </header>

        {/* Body */}
        <div className="flex-1 overflow-y-auto px-6 py-4">
          {runErrors.length > 0 && (
            <div className="mb-4 space-y-2">
              {runErrors.map((err) => (
                <div
                  key={err.id}
                  className="flex items-start justify-between gap-3 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-600 dark:border-red-800 dark:bg-red-950 dark:text-red-400"
                >
                  <span>{err.message}</span>
                  <button
                    onClick={() => dismissError(err.id)}
                    className="shrink-0 rounded p-0.5 text-red-500 hover:bg-red-100 dark:text-red-400 dark:hover:bg-red-900"
                    aria-label="Dismiss"
                  >
                    <X size={14} />
                  </button>
                </div>
              ))}
            </div>
          )}

          <StatusTab
            plugins={visiblePlugins}
            operations={operations}
            loading={loading}
            runningPlugins={runningPlugins}
            onRun={handleRunSingle}
            onViewError={(plugin, error) => setSelectedError({ plugin, error })}
            schedules={schedules}
            onScheduleChange={(plugin, expression, enabled) =>
              scheduleMutation.mutate({ plugin, expression, enabled })
            }
            scheduleSaving={scheduleMutation.isPending}
          />
        </div>
      </div>

      <PluginErrorModal
        pluginName={selectedError?.plugin ?? ''}
        error={selectedError?.error ?? null}
        onClose={() => setSelectedError(null)}
      />
    </div>
  );
}

function StatusTab({
  plugins,
  operations,
  loading,
  runningPlugins,
  onRun,
  onViewError,
  schedules,
  onScheduleChange,
  scheduleSaving,
}: {
  plugins: string[];
  operations: OperationResource[];
  loading: boolean;
  runningPlugins: Set<string>;
  onRun: (plugin: string) => void;
  onViewError: (plugin: string, error: StatusErrorResource) => void;
  schedules: Map<string, ScheduleResource>;
  onScheduleChange: (plugin: string, expression: string, enabled: boolean) => void;
  scheduleSaving: boolean;
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
    <table className="w-full table-fixed text-left text-sm">
      <colgroup>
        <col className="w-[22%]" />
        <col className="w-[15%]" />
        <col className="w-[12%]" />
        <col className="w-[17%]" />
        <col className="w-[20%]" />
        <col className="w-[18%]" />
        <col className="w-[11%]" />
      </colgroup>
      <thead>
        <tr className="border-b border-gray-200 text-xs uppercase tracking-wide text-gray-500 dark:border-gray-700 dark:text-gray-400">
          <th className="py-2 pr-4 font-medium">Plugin</th>
          <th className="py-2 pr-4 font-medium">Last run</th>
          <th className="py-2 pr-4 font-medium">Duration</th>
          <th className="py-2 pr-4 font-medium">Status</th>
          <th className="py-2 pr-4 font-medium">Result</th>
          <th className="py-2 pr-4 font-medium">Schedule</th>
          <th className="py-2 text-right font-medium">Action</th>
        </tr>
      </thead>
      <tbody>
        {plugins.map((plugin) => {
          const op = latestByPlugin.get(plugin);
          // Hide previous status/result while running to avoid displaying stale data
          // next to an active spinner. Resets as soon as the plugin completes.
          const running = runningPlugins.has(plugin);

          return (
            <tr key={plugin} className="border-b border-gray-100 last:border-0 dark:border-gray-800">
              <td className="py-3 pr-4 font-medium text-gray-900 dark:text-gray-100">{plugin}</td>
              <td className="py-3 pr-4 text-gray-500 dark:text-gray-400">
                {op ? formatRelativeTime(op.createdAt) : 'Never'}
              </td>
              <td className="py-3 pr-4 text-gray-500 dark:text-gray-400">
                {running ? (
                  <span className="text-xs text-gray-400">—</span>
                ) : op?.startTime && op?.endTime ? (
                  formatDuration(op.startTime, op.endTime)
                ) : (
                  <span className="text-xs text-gray-400">—</span>
                )}
              </td>
              <td className="py-3 pr-4">
                {running ? (
                  <span className="text-xs text-gray-400">—</span>
                ) : op && op.state === 'FAILED' && op.error ? (
                  <button
                    onClick={() => onViewError(plugin, op.error!)}
                    className="inline-flex items-center gap-1.5 rounded bg-red-50 px-2 py-1 text-xs font-medium text-red-700 hover:bg-red-100 dark:bg-red-950/80 dark:text-red-400 dark:hover:bg-red-900"
                  >
                    <AlertCircle size={13} />
                    <span>Failed</span>
                    <span className="underline ml-0.5">Details</span>
                  </button>
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
                ) : (
                  <span className="text-xs text-gray-400">—</span>
                )}
              </td>
              <td className="py-3 pr-4">
                <ScheduleEditor
                  plugin={plugin}
                  schedule={schedules.get(plugin)}
                  disabled={scheduleSaving}
                  onSave={onScheduleChange}
                />
              </td>
              <td className="py-3 text-right">
                <button
                  onClick={() => onRun(plugin)}
                  disabled={running}
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

function ScheduleEditor({
  plugin,
  schedule,
  disabled,
  onSave,
}: {
  plugin: string;
  schedule?: ScheduleResource;
  disabled: boolean;
  onSave: (plugin: string, expression: string, enabled: boolean) => void;
}) {
  const currentExpression = schedule?.expression ?? '';
  const currentEnabled = schedule?.enabled ?? false;
  const [expression, setExpression] = useState(currentExpression);
  const [enabled, setEnabled] = useState(currentEnabled);

  useEffect(() => {
    setExpression(currentExpression);
    setEnabled(currentEnabled);
  }, [currentExpression, currentEnabled]);
  const dirty = expression !== currentExpression || enabled !== currentEnabled;

  return (
    <div className="flex items-center gap-1.5">
      <input
        value={expression}
        onChange={(event) => setExpression(event.target.value)}
        placeholder="manual"
        aria-label={`Schedule for ${plugin}`}
        className="w-28 rounded border border-gray-300 bg-white px-2 py-1 text-xs dark:border-gray-600 dark:bg-gray-800"
        disabled={disabled}
      />
      <label className="flex items-center gap-1 text-xs text-gray-500">
        <input
          type="checkbox"
          checked={enabled}
          onChange={(event) => setEnabled(event.target.checked)}
          disabled={disabled}
        />
        on
      </label>
      {dirty && (
        <button
          type="button"
          onClick={() => onSave(plugin, expression, enabled)}
          disabled={disabled}
          className="rounded bg-primary px-2 py-1 text-xs text-white disabled:opacity-50"
        >
          Save
        </button>
      )}
    </div>
  );
}
