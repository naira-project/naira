import { Clock, RefreshCw } from 'lucide-react';
import { OperationResource } from '../lib/catalogApi';
import { PluginStatusBadge } from './PluginStatusBadge';
import { PluginErrorLog } from './PluginErrorLog';

interface PluginRunHistoryProps {
  operations: OperationResource[];
  loading: boolean;
  onRefresh: () => void;
}

/**
 * Table showing past plugin run executions.
 * Each entry shows: plugin name, timestamp, status badge, and expandable error log.
 */
export function PluginRunHistory({ operations, loading, onRefresh }: PluginRunHistoryProps) {
  if (operations.length === 0 && !loading) {
    return (
      <div className="rounded-md border border-gray-200 p-4 text-center text-sm text-gray-500 dark:border-gray-700 dark:text-gray-400">
        <Clock size={24} className="mx-auto mb-2 opacity-40" />
        <p>No plugin runs yet. Trigger a run to see history.</p>
      </div>
    );
  }

  return (
    <div className="rounded-md border border-gray-200 dark:border-gray-700">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-gray-200 px-4 py-2 dark:border-gray-700">
        <h3 className="text-sm font-semibold text-gray-700 dark:text-gray-300">
          Plugin Run History
        </h3>
        <button
          onClick={onRefresh}
          disabled={loading}
          className="inline-flex items-center gap-1 text-xs text-gray-500 hover:text-gray-700 disabled:opacity-50 dark:text-gray-400 dark:hover:text-gray-200"
        >
          <RefreshCw size={12} className={loading ? 'animate-spin' : ''} />
          Refresh
        </button>
      </div>

      {/* List */}
      <div className="divide-y divide-gray-100 dark:divide-gray-800">
        {operations.map((op) => (
          <div key={op.name} className="px-4 py-3">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <span className="text-sm font-medium text-gray-900 dark:text-gray-100">
                  {op.plugin}
                </span>
                <PluginStatusBadge state={op.state} />
              </div>
              <span className="text-xs text-gray-500 dark:text-gray-400">
                {formatTimestamp(op.createdAt)}
              </span>
            </div>

            {/* Result summary for succeeded operations */}
            {op.state === 'SUCCEEDED' && (
              <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {op.nodesUpserted} node(s), {op.relationsUpserted} relation(s) upserted
              </p>
            )}

            {/* Error details for failed operations */}
            {op.state === 'FAILED' && op.error && (
              <div className="mt-2">
                <PluginErrorLog error={op.error} />
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}

function formatTimestamp(iso: string): string {
  const date = new Date(iso);
  return date.toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });
}