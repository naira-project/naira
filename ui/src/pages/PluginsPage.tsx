import { useQuery } from '@tanstack/react-query';
import cronstrue from 'cronstrue';
import { AlertCircle, Play, RefreshCw, X } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useSearchParams } from 'react-router';
import { PluginErrorModal } from '../components/PluginErrorModal';
import { PluginStatusBadge } from '../components/PluginStatusBadge';
import { useOpenMFPContext } from '../hooks/useOpenMFPContext';
import { usePluginsStatus } from '../hooks/usePluginOperations';
import {
  fetchSchedules,
  type OperationResource,
  type ScheduleResource,
  type StatusErrorResource,
} from '../lib/catalogApi';
import { queryKeys } from '../lib/queryKeys';
import { formatDuration, formatRelativeTime, latestOperationPerPlugin } from '../lib/utils';

/** Dedicated page for managing plugin ingestion and schedules. */
export default function PluginsPage() {
  const [searchParams] = useSearchParams();
  const only = searchParams.get('only');
  const allowedPlugins = only
    ? only
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean)
    : undefined;

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
  const schedulesQuery = useQuery({
    queryKey: queryKeys.schedules,
    queryFn: () => fetchSchedules(token),
  });
  const schedules = useMemo(
    () => new Map((schedulesQuery.data ?? []).map((s) => [s.plugin, s])),
    [schedulesQuery.data],
  );
  const visiblePlugins = allowedPlugins
    ? plugins.filter((p) => allowedPlugins.includes(p))
    : plugins;

  const [selectedError, setSelectedError] = useState<{
    plugin: string;
    error: StatusErrorResource;
  } | null>(null);

  const handleRunVisible = async () => {
    allowedPlugins ? runSubset(visiblePlugins) : runAll();
  };

  return (
    <div className="flex h-screen overflow-hidden bg-background">
      <div className="flex flex-1 flex-col overflow-hidden">
        {/* Top bar */}
        <header className="flex shrink-0 items-center gap-3 border-b border-gray-200 bg-card px-6 py-3">
          <div>
            <h1 className="text-lg font-semibold text-foreground">Plugins & Ingestion</h1>
            <p className="text-xs text-muted-foreground">
              Run individual plugins or all at once, and inspect their latest status.
            </p>
          </div>
          <div className="flex-1" />
          <button
            type="button"
            onClick={refresh}
            disabled={loading}
            className="inline-flex items-center justify-center rounded-md p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 disabled:cursor-not-allowed disabled:opacity-50"
            title="Refresh status"
            aria-label="Refresh status"
          >
            <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
          </button>
          <button
            type="button"
            onClick={handleRunVisible}
            disabled={runAllActive}
            className="inline-flex items-center gap-2 rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-white disabled:opacity-60"
          >
            <RefreshCw size={14} className={runAllActive ? 'animate-spin' : ''} />
            {runAllActive ? 'Running…' : allowedPlugins ? 'Run Shown Plugins' : 'Run All Plugins'}
          </button>
        </header>
        <div className="flex-1 overflow-y-auto px-6 py-4">
          {runErrors.length > 0 && (
            <div className="mb-4 space-y-2">
              {runErrors.map((err) => (
                <div
                  key={err.id}
                  className="flex items-start justify-between gap-3 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-600"
                >
                  <span>{err.message}</span>
                  <button
                    type="button"
                    onClick={() => dismissError(err.id)}
                    className="shrink-0 rounded p-0.5 text-red-500 hover:bg-red-100"
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
            onRun={runOne}
            onViewError={(plugin, error) => setSelectedError({ plugin, error })}
            schedules={schedules}
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

interface StatusTabProps {
  plugins: string[];
  operations: OperationResource[];
  loading: boolean;
  runningPlugins: Set<string>;
  onRun: (plugin: string) => void;
  onViewError: (plugin: string, error: StatusErrorResource) => void;
  schedules: Map<string, ScheduleResource>;
}

function StatusTab({
  plugins,
  operations,
  loading,
  runningPlugins,
  onRun,
  onViewError,
  schedules,
}: StatusTabProps) {
  const latestByPlugin = latestOperationPerPlugin(operations);

  if (plugins.length === 0 && !loading) {
    return (
      <p className="py-8 text-center text-sm text-gray-500">
        No plugins registered.
      </p>
    );
  }

  return (
    <table className="w-full table-fixed text-left text-sm">
      <colgroup>
        <col className="w-[20%]" />
        <col className="w-[14%]" />
        <col className="w-[11%]" />
        <col className="w-[15%]" />
        <col className="w-[18%]" />
        <col className="w-[12%]" />
        <col className="w-[10%]" />
      </colgroup>
      <thead>
        <tr className="border-b border-gray-200 text-xs uppercase tracking-wide text-gray-500">
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
          const running = runningPlugins.has(plugin);

          return (
            <tr
              key={plugin}
              className="border-b border-gray-100 last:border-0"
            >
              <td className="py-3 pr-4 font-medium text-gray-900">{plugin}</td>
              <td className="py-3 pr-4 text-gray-500">
                {op ? formatRelativeTime(op.createdAt) : 'Never'}
              </td>
              <td className="py-3 pr-4 text-gray-500">
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
                    type="button"
                    onClick={() => {
                      if (op.error) {
                        onViewError(plugin, op.error);
                      }
                    }}
                    className="inline-flex items-center gap-1.5 rounded bg-red-50 px-2 py-1 text-xs font-medium text-red-700 hover:bg-red-100"
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
              <td className="py-3 pr-4 text-gray-500">
                {running ? (
                  <span className="text-xs text-gray-400">—</span>
                ) : op && op.state === 'SUCCEEDED' ? (
                  `${op.nodesUpserted} node(s), ${op.relationsUpserted} relation(s)`
                ) : (
                  <span className="text-xs text-gray-400">—</span>
                )}
              </td>
              <td className="py-3 pr-4">
                <ScheduleCell schedule={schedules.get(plugin)} />
              </td>
              <td className="py-3 text-right">
                <button
                  type="button"
                  onClick={() => onRun(plugin)}
                  disabled={running}
                  className="inline-flex items-center gap-1.5 rounded-md bg-primary px-2.5 py-1.5 text-xs font-medium text-white disabled:opacity-50"
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

function ScheduleCell({ schedule }: { schedule?: ScheduleResource }) {
  const friendly =
    schedule?.expression && schedule.enabled
      ? friendlySchedule(schedule.expression)
      : 'Not scheduled';

  return (
    <div className="max-w-full" title={schedule?.expression ?? undefined}>
      <span className={friendly === 'Not scheduled' ? 'text-gray-400' : 'text-gray-700'}>
        {friendly}
      </span>
    </div>
  );
}

function friendlySchedule(expression: string): string {
  try {
    return cronstrue.toString(expression, { use24HourTimeFormat: true });
  } catch {
    return 'Custom schedule';
  }
}
