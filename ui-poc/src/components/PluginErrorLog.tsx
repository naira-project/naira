import { useState } from 'react';
import { AlertTriangle, ChevronDown, ChevronRight } from 'lucide-react';
import { StatusErrorResource } from '../lib/catalogApi';

interface PluginErrorLogProps {
  error: StatusErrorResource;
}

/**
 * Expandable panel showing error details for a failed plugin run.
 * Displays the AIP-193 error code and message.
 */
export function PluginErrorLog({ error }: PluginErrorLogProps) {
  const [expanded, setExpanded] = useState(false);

  return (
    <div className="rounded-md border border-red-200 bg-red-50 text-sm dark:border-red-800 dark:bg-red-950">
      <button
        onClick={() => setExpanded(!expanded)}
        className="flex w-full items-center gap-2 px-3 py-2 text-left text-red-700 hover:bg-red-100 dark:text-red-400 dark:hover:bg-red-900"
      >
        {expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        <AlertTriangle size={14} />
        <span className="font-medium">Error (code {error.code})</span>
      </button>
      {expanded && (
        <pre className="overflow-x-auto px-3 pb-2 pt-1 font-mono text-xs text-red-600 dark:text-red-300">
          {error.message}
        </pre>
      )}
    </div>
  );
}