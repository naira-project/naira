import { cn } from '../lib/utils';
import { Button } from './ui/button';

interface PluginTabsProps {
  plugins: string[];
  activePlugin: string | null;
  onSelect: (plugin: string | null) => void;
}

/**
 * Underline-style tab bar shown below the kind selector: an "All" tab plus one
 * tab per plugin that has claimed at least one node of the active kind.
 * Selecting a plugin filters the table to nodes claimed by that plugin.
 */
export default function PluginTabs({ plugins, activePlugin, onSelect }: PluginTabsProps) {
  return (
    <div className="flex gap-1 border-b border-gray-200">
      <Button
        type="button"
        variant="ghost"
        size="sm"
        onClick={() => onSelect(null)}
        className={cn(
          'rounded-none px-3 py-1.5 text-sm transition-colors',
          activePlugin === null
            ? 'border-b-2 border-primary font-semibold text-foreground'
            : 'text-muted-foreground hover:text-foreground',
        )}
      >
        All
      </Button>
      {plugins.map((plugin) => (
        <Button
          type="button"
          key={plugin}
          variant="ghost"
          size="sm"
          onClick={() => onSelect(plugin)}
          className={cn(
            'rounded-none px-3 py-1.5 text-sm transition-colors',
            activePlugin === plugin
              ? 'border-b-2 border-primary font-semibold text-foreground'
              : 'text-muted-foreground hover:text-foreground',
          )}
        >
          {plugin}
        </Button>
      ))}
    </div>
  );
}
