import { Loader2, Play } from 'lucide-react';

interface PluginRunButtonProps {
  pluginName: string;
  running: boolean;
  onRun: () => void;
}

/**
 * Per-plugin run button with loading spinner and disabled state.
 * Shows a play icon when idle and a spinning loader when running.
 */
export function PluginRunButton({ pluginName, running, onRun }: PluginRunButtonProps) {
  return (
    <button
      onClick={onRun}
      disabled={running}
      className="inline-flex items-center gap-1.5 rounded-md bg-primary px-2.5 py-1.5 text-xs font-medium text-white transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
      title={`Run ${pluginName} plugin`}
    >
      {running ? <Loader2 size={12} className="animate-spin" /> : <Play size={12} />}
      {running ? 'Running…' : 'Run'}
    </button>
  );
}