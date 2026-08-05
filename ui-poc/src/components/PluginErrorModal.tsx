import { useState } from 'react';
import { X, AlertTriangle, Copy, Check } from 'lucide-react';
import { StatusErrorResource } from '../lib/catalogApi';

interface PluginErrorModalProps {
  pluginName: string;
  error: StatusErrorResource | null;
  onClose: () => void;
}

export function PluginErrorModal({ pluginName, error, onClose }: PluginErrorModalProps) {
  const [copied, setCopied] = useState(false);

  if (!error) return null;

  const handleCopy = () => {
    navigator.clipboard.writeText(`Error: ${error.message}`);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div
      className="fixed inset-0 z-[60] flex items-center justify-center bg-black/60 p-4"
      onClick={onClose}
      role="dialog"
      aria-modal="true"
    >
      <div
        className="flex max-h-[85vh] w-full max-w-xl flex-col rounded-lg bg-white shadow-2xl dark:bg-background-dark-paper"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex shrink-0 items-center justify-between border-b border-gray-200 px-5 py-4 dark:border-gray-700">
          <div className="flex items-center gap-2.5 text-red-600 dark:text-red-400">
            <AlertTriangle size={20} className="shrink-0" />
            <h3 className="text-base font-semibold text-foreground dark:text-foreground-dark-default">
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
          <div className="flex items-center justify-end rounded-md bg-red-50 px-3 py-2 text-xs font-medium text-red-700 dark:bg-red-950/60 dark:text-red-300">
            <button
              onClick={handleCopy}
              className="inline-flex items-center gap-1 rounded bg-white/80 px-2 py-1 text-xs font-medium text-red-700 shadow-sm transition-colors hover:bg-white dark:bg-red-900/50 dark:text-red-200 dark:hover:bg-red-900"
            >
              {copied ? <Check size={12} /> : <Copy size={12} />}
              {copied ? 'Copied' : 'Copy log'}
            </button>
          </div>

          <div>
            <label className="mb-1.5 block text-xs font-medium text-gray-500 dark:text-gray-400">
              Error Details / Traceback
            </label>
            <div className="max-h-80 overflow-y-auto rounded-md border border-gray-200 bg-gray-900 p-3 font-mono text-xs leading-relaxed text-red-400 dark:border-gray-700">
              <pre className="whitespace-pre-wrap break-words">{error.message}</pre>
            </div>
          </div>
        </div>

        {/* Footer */}
        <div className="flex shrink-0 justify-end border-b border-gray-200 bg-gray-50 px-5 py-3 dark:border-gray-700 dark:bg-gray-900/30">
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