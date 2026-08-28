import { cn } from '../lib/utils';

interface KindSelectorProps {
  kinds: string[];
  activeKind: string | null;
  onSelect: (kind: string) => void;
  loading?: boolean;
}

/**
 * A row of pill-shaped buttons: one per discovered kind.
 * The active kind is highlighted with the primary colour.
 */
export default function KindSelector({ kinds, activeKind, onSelect, loading }: KindSelectorProps) {
  if (loading) {
    return (
      <div className="flex gap-2">
        <div className="h-8 w-24 animate-pulse rounded-lg bg-gray-200" />
        <div className="h-8 w-20 animate-pulse rounded-lg bg-gray-200" />
        <div className="h-8 w-28 animate-pulse rounded-lg bg-gray-200" />
      </div>
    );
  }

  if (kinds.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">No kinds discovered. Synchronize data first.</p>
    );
  }

  return (
    <div className="flex flex-wrap gap-2">
      {kinds.map((kind) => (
        <button
          key={kind}
          onClick={() => onSelect(kind)}
          className={cn(
            'rounded-lg px-4 py-1.5 text-sm transition-colors',
            activeKind === kind
              ? 'bg-primary font-semibold text-white'
              : 'bg-gray-100 font-normal text-foreground hover:bg-gray-200',
          )}
        >
          {kind}
        </button>
      ))}
    </div>
  );
}
