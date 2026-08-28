import { X } from 'lucide-react';
import type { StatusErrorResource } from '../lib/catalogApi';

interface PluginErrorModalProps {
  pluginName: string;
  error: StatusErrorResource | null;
  onClose: () => void;
}

export function PluginErrorModal({ pluginName, error, onClose }: PluginErrorModalProps) {
  if (!error) return null;

  return (
    <div
      className="fixed inset-0 z-[60] flex items-center justify-center bg-black/60 p-4"
      onClick={onClose}
      role="dialog"
      aria-modal="true"
    >
      <div
        className="flex max-h-[85vh] w-full max-w-xl flex-col rounded-lg bg-card shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex shrink-0 items-center justify-between border-b border-gray-200 px-5 py-4 dark:border-gray-700">
          <div className="flex items-center gap-2.5 dark:text-red-400">
            <h3 className="text-base font-semibold text-foreground">
              Execution Error: <span className="font-mono text-sm font-normal">{pluginName}</span>
            </h3>
          </div>
          <button
            onClick={onClose}
            className="rounded-md p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-200"
            aria-label="Close error log"
          >
            <X size={18} />
          </button>
        </div>

        {/* Content */}
        <div className="space-y-4 overflow-y-auto p-5">
          <div>
            <label className="mb-1.5 block text-xs font-medium text-gray-500 dark:text-gray-400">
              Error Details
            </label>
            <div className="max-h-80 overflow-y-auto rounded-md border border-gray-200 bg-gray-900 p-3 font-mono text-xs leading-relaxed text-red-400 dark:border-gray-700">
              <pre className="whitespace-pre-wrap break-words">{error.message}</pre>
            </div>
          </div>
        </div>

        {/* Footer */}
        <div className="flex shrink-0 justify-end border-t border-gray-200 bg-gray-50 px-5 py-3 dark:border-gray-700 dark:bg-gray-900/30">
          <button
            onClick={onClose}
            className="rounded-md bg-gray-200 px-4 py-1.5 text-xs font-medium text-gray-700 hover:bg-gray-300 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-gray-700"
          >
            Close
          </button>
        </div>
      </div>
    </div>
  );
}
