import { useEffect, useMemo, useState } from 'react';
import { useSearchParams } from 'react-router';
import cronstrue from 'cronstrue';
import { X, Play, RefreshCw, AlertCircle, Pencil, X as Close } from 'lucide-react';
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

/** Dedicated page for managing plugin ingestion and schedules. */
export default function PluginsPage() {
  const [searchParams] = useSearchParams();
  const only = searchParams.get('only');
  const allowedPlugins = only
    ? only.split(',').map((s) => s.trim()).filter(Boolean)
    : undefined;

  const {
    plugins, operations, loading, runningPlugins, runAllActive, runErrors,
    dismissError, refresh, runOne, runAll, runSubset,
  } = usePluginsStatus();
  const { token } = useOpenMFPContext();
  const queryClient = useQueryClient();
  const schedulesQuery = useQuery({
    queryKey: queryKeys.schedules,
    queryFn: () => fetchSchedules(token),
  });
  const scheduleMutation = useMutation({
    mutationFn: ({
      plugin, expression, enabled,
    }: { plugin: string; expression: string; enabled: boolean }) =>
      updateSchedule(token, plugin, { expression: expression || undefined, enabled }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.schedules }),
  });
  const schedules = useMemo(
    () => new Map((schedulesQuery.data ?? []).map((s) => [s.plugin, s])),
    [schedulesQuery.data],
  );
  const visiblePlugins = allowedPlugins
    ? plugins.filter((p) => allowedPlugins.includes(p))
    : plugins;
  const [selectedError, setSelectedError] = useState<{ plugin: string; error: StatusErrorResource } | null>(null);
  const [editingSchedule, setEditingSchedule] = useState<string | null>(null);

  const handleRunVisible = async () => {
    allowedPlugins ? runSubset(visiblePlugins) : runAll();
  };

  return (
    <div className="flex h-screen overflow-hidden bg-background dark:bg-background-dark-default">
      <div className="flex flex-1 flex-col overflow-hidden">
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
            className="inline-flex items-center justify-center rounded-md p-1.5 text-gray-500 hover:bg-gray-100 disabled:opacity-50"
            title="Refresh status"
            aria-label="Refresh status"
          >
            <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
          </button>
          <button
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
                  <button onClick={() => dismissError(err.id)} aria-label="Dismiss">
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
            onEditSchedule={setEditingSchedule}
          />
        </div>
      </div>
      <PluginErrorModal
        pluginName={selectedError?.plugin ?? ''}
        error={selectedError?.error ?? null}
        onClose={() => setSelectedError(null)}
      />
      {editingSchedule && (
        <ScheduleDialog
          plugin={editingSchedule}
          schedule={schedules.get(editingSchedule)}
          disabled={scheduleMutation.isPending}
          onClose={() => setEditingSchedule(null)}
          onSave={(expression, enabled) =>
            scheduleMutation.mutate(
              { plugin: editingSchedule, expression, enabled },
              { onSuccess: () => setEditingSchedule(null) },
            )
          }
        />
      )}
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
  onEditSchedule: (plugin: string) => void;
}

function StatusTab({
  plugins, operations, loading, runningPlugins, onRun, onViewError, schedules, onEditSchedule,
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
        <col className="w-[22%]" />
        <col className="w-[16%]" />
        <col className="w-[12%]" />
        <col className="w-[17%]" />
        <col className="w-[22%]" />
        <col className="w-[11%]" />
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
            <tr key={plugin} className="border-b border-gray-100 last:border-0">
              <td className="py-3 pr-4 font-medium">{plugin}</td>
              <td className="py-3 pr-4 text-gray-500">
                {op ? formatRelativeTime(op.createdAt) : 'Never'}
              </td>
              <td className="py-3 pr-4 text-gray-500">
                {running
                  ? '—'
                  : op?.startTime && op?.endTime
                    ? formatDuration(op.startTime, op.endTime)
                    : '—'}
              </td>
              <td className="py-3 pr-4">
                {running
                  ? '—'
                  : op?.state === 'FAILED' && op?.error
                    ? (
                      <button
                        onClick={() => onViewError(plugin, op.error!)}
                        className="inline-flex items-center gap-1.5 rounded bg-red-50 px-2 py-1 text-xs text-red-700"
                      >
                        <AlertCircle size={13} />
                        Failed
                      </button>
                    )
                    : op
                      ? <PluginStatusBadge state={op.state} />
                      : <span className="text-xs text-gray-400">Not run yet</span>}
              </td>
              <td className="py-3 pr-4 text-gray-500">
                {running
                  ? '—'
                  : op?.state === 'SUCCEEDED'
                    ? `${op.nodesUpserted} node(s), ${op.relationsUpserted} relation(s)`
                    : '—'}
              </td>
              <td className="py-3 pr-4">
                <ScheduleCell
                  plugin={plugin}
                  schedule={schedules.get(plugin)}
                  onClick={() => onEditSchedule(plugin)}
                />
              </td>
              <td className="py-3 text-right">
                <button
                  onClick={() => onRun(plugin)}
                  disabled={running}
                  className="inline-flex items-center gap-1.5 rounded-md bg-primary px-2.5 py-1.5 text-xs font-medium text-white disabled:opacity-50"
                >
                  {running
                    ? <RefreshCw size={12} className="animate-spin" />
                    : <Play size={12} />}
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

function ScheduleCell({
  plugin, schedule, onClick,
}: {
  plugin: string;
  schedule?: ScheduleResource;
  onClick: () => void;
}) {
  const friendly = schedule?.expression && schedule.enabled
    ? friendlySchedule(schedule.expression)
    : 'Not scheduled';

  return (
    <button
      type="button"
      onClick={onClick}
      className="group flex max-w-full items-center gap-2 text-left"
      title="Edit schedule"
    >
      <span className={friendly === 'Not scheduled' ? 'text-gray-400' : 'text-gray-700'}>
        {friendly}
      </span>
      <Pencil size={13} className="shrink-0 text-gray-400 opacity-0 group-hover:opacity-100" />
    </button>
  );
}

function ScheduleDialog({
  plugin,
  schedule,
  disabled,
  onClose,
  onSave,
}: {
  plugin: string;
  schedule?: ScheduleResource;
  disabled: boolean;
  onClose: () => void;
  onSave: (expression: string, enabled: boolean) => void;
}) {
  const initialExpression = schedule?.expression ?? '';
  const initialEnabled = schedule?.enabled ?? false;
  const [expression, setExpression] = useState(initialExpression);
  const [enabled, setEnabled] = useState(initialEnabled);
  const preset = schedulePreset(expression, enabled);
  const dirty = expression !== initialExpression || enabled !== initialEnabled;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4"
      role="dialog"
      aria-modal="true"
      aria-labelledby="schedule-dialog-title"
    >
      <div className="w-full max-w-md rounded-lg bg-white p-5 shadow-xl dark:bg-gray-900">
        <div className="mb-4 flex items-center justify-between">
          <h2
            id="schedule-dialog-title"
            className="text-base font-semibold"
          >
            Edit schedule: {plugin}
          </h2>
          <button type="button" onClick={onClose} aria-label="Close">
            <Close size={16} />
          </button>
        </div>

        <label className="mb-3 block text-sm">
          Schedule
          <select
            value={preset}
            onChange={(event) => applyPreset(event.target.value, setExpression, setEnabled)}
            disabled={disabled}
            className="mt-1 w-full rounded border px-3 py-2 text-sm"
          >
            <option value="manual">Manual only</option>
            <option value="minutes">Every N minutes</option>
            <option value="hourly">Every hour</option>
            <option value="daily">Every day</option>
            <option value="custom">Custom</option>
          </select>
        </label>

        {preset === 'minutes' && (
          <label className="mb-3 block text-sm">
            Every
            {' '}
            <input
              type="number"
              min="1"
              value={minutesFromCron(expression)}
              onChange={(event) =>
                setExpression(`*/${Math.max(1, Number(event.target.value) || 1)} * * * *`)}
              className="mx-1 w-20 rounded border px-2 py-1"
              disabled={disabled}
            />
            {' '}
            minutes
          </label>
        )}

        {preset === 'hourly' && (
          <label className="mb-3 block text-sm">
            At minute
            {' '}
            <select
              value={minuteFromHourlyCron(expression)}
              onChange={(event) => setExpression(`${event.target.value} * * * *`)}
              className="ml-2 rounded border px-2 py-1"
              disabled={disabled}
            >
              {['00', '15', '30', '45'].map((minute) => (
                <option key={minute} value={Number(minute)}>
                  {minute}
                </option>
              ))}
            </select>
          </label>
        )}

        {preset === 'daily' && (
          <label className="mb-3 block text-sm">
            At
            {' '}
            <input
              type="time"
              value={timeFromCron(expression)}
              onChange={(event) => setExpression(timeToCron(event.target.value))}
              className="ml-2 rounded border px-2 py-1"
              disabled={disabled}
            />
          </label>
        )}

        {preset === 'custom' && (
          <label className="mb-3 block text-sm">
            Cron expression
            <input
              value={expression}
              onChange={(event) => setExpression(event.target.value)}
              placeholder="*/15 * * * *"
              className="mt-1 w-full rounded border px-3 py-2 text-sm font-mono"
              disabled={disabled}
            />
          </label>
        )}

        <div className="mb-4 text-xs text-gray-500">
          {expression && enabled ? friendlySchedule(expression) : 'Ingestion is disabled.'}
        </div>

        <div className="flex items-center justify-between">
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={enabled}
              onChange={(event) => setEnabled(event.target.checked)}
              disabled={disabled}
              className="rounded"
            />
            Enabled
          </label>

          <div className="flex gap-2">
            <button
              type="button"
              onClick={onClose}
              disabled={disabled}
              className="rounded-md border border-gray-300 px-3 py-1.5 text-sm hover:bg-gray-50"
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={() => onSave(expression, enabled)}
              disabled={disabled || !dirty}
              className="rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-white disabled:opacity-60"
            >
              Save
            </button>
          </div>
        </div>
      </div>
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

function schedulePreset(expression: string, enabled: boolean): string {
  if (!enabled || !expression) return 'manual';
  if (/^\*\/\d+ \* \* \* \*$/.test(expression)) return 'minutes';
  if (/^\d+ \* \* \* \*$/.test(expression)) return 'hourly';
  if (/^\d+ \d+ \* \* \*$/.test(expression)) return 'daily';
  return 'custom';
}

function minutesFromCron(expression: string): number {
  const match = expression.match(/^\*\/(\d+) /);
  return match ? Number(match[1]) : 15;
}

function minuteFromHourlyCron(expression: string): string {
  const match = expression.match(/^(\d+) \* /);
  return match?.[1] ?? '0';
}

function timeFromCron(expression: string): string {
  const match = expression.match(/^(\d+) (\d+) /);
  return match
    ? `${match[2].padStart(2, '0')}:${match[1].padStart(2, '0')}`
    : '03:00';
}

function timeToCron(time: string): string {
  const [hour = '0', minute = '0'] = time.split(':');
  return `${Number(minute)} ${Number(hour)} * * *`;
}

function applyPreset(
  preset: string,
  setExpression: (value: string) => void,
  setEnabled: (value: boolean) => void,
) {
  if (preset === 'manual') {
    setExpression('');
    setEnabled(false);
    return;
  }
  setEnabled(true);
  if (preset === 'minutes') setExpression('*/15 * * * *');
  if (preset === 'hourly') setExpression('0 * * * *');
  if (preset === 'daily') setExpression('0 3 * * *');
}
