import { useState } from 'react';
import { ChevronDown, ChevronRight, Copy } from 'lucide-react';
import { isComplexValue } from '../lib/kindUtils';

interface PropertiesPanelProps {
  props: Record<string, unknown>;
  title?: string;
}

/**
 * Renders a node's `props` as a key-value list.
 * Complex values (objects / arrays) are rendered as collapsible JSON trees.
 */
export default function PropertiesPanel({ props, title = 'Properties' }: PropertiesPanelProps) {
  const keys = Object.keys(props).sort();

  if (keys.length === 0) {
    return (
      <div className="text-sm text-foreground-secondary dark:text-foreground-dark-secondary">
        No properties.
      </div>
    );
  }

  return (
    <div>
      <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-foreground-secondary dark:text-foreground-dark-secondary">
        {title}
      </h3>
      <div className="divide-y divide-gray-200 rounded-lg border border-gray-200 dark:divide-gray-700 dark:border-gray-700">
        {keys.map((key) => (
          <PropertyRow key={key} name={key} value={props[key]} />
        ))}
      </div>
    </div>
  );
}

function PropertyRow({ name, value }: { name: string; value: unknown }) {
  const [expanded, setExpanded] = useState(false);
  const isComplex = isComplexValue(value);

  return (
    <div className="group px-4 py-2.5 text-sm hover:bg-gray-50 dark:hover:bg-white/5">
      <div className="flex items-start gap-3">
        {/* Key */}
        <span className="min-w-[160px] max-w-[200px] shrink-0 font-mono text-xs font-medium text-foreground-secondary dark:text-foreground-dark-secondary">
          {name}
        </span>

        {/* Value */}
        <div className="flex-1">
          {isComplex ? (
            <button
              onClick={() => setExpanded((v) => !v)}
              className="flex items-center gap-1 text-xs text-foreground-secondary hover:text-foreground dark:text-foreground-dark-secondary dark:hover:text-foreground-dark-default"
            >
              {expanded ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
              {expanded ? 'Collapse' : 'Expand'}
              <span className="ml-1 font-mono text-[0.6rem] opacity-60">
                {Array.isArray(value) ? `[${(value as unknown[]).length}]` : `{${Object.keys(value as Record<string, unknown>).length}}`}
              </span>
            </button>
          ) : (
            <span className="font-mono text-xs text-foreground dark:text-foreground-dark-default">
              {String(value ?? '—')}
            </span>
          )}
        </div>

        {/* Copy button */}
        <button
          onClick={() => {
            const text = isComplex ? JSON.stringify(value, null, 2) : String(value ?? '');
            navigator.clipboard.writeText(text).catch(() => {});
          }}
          className="shrink-0 opacity-0 group-hover:opacity-100 transition-opacity"
          title="Copy value"
        >
          <Copy size={14} className="text-foreground-secondary hover:text-foreground dark:text-foreground-dark-secondary dark:hover:text-foreground-dark-default" />
        </button>
      </div>

      {/* Expanded complex value */}
      {isComplex && expanded && (
        <pre className="ml-[172px] mt-2 overflow-x-auto rounded-md bg-gray-100 p-3 text-xs dark:bg-white/10">
          {JSON.stringify(value, null, 2)}
        </pre>
      )}
    </div>
  );
}
