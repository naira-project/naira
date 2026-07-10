import { useState } from 'react';
import { ChevronDown, ChevronRight, Copy } from 'lucide-react';
import { isComplexValue, isUrlValue, tryParseJson } from '../lib/kindUtils';

type PropertiesLayout = 'row' | 'stacked';

interface PropertiesPanelProps {
  props: Record<string, unknown>;
  title?: string;
  /**
   * 'row'     – key and value side by side. Used in the standalone Properties view,
   *             where there's enough horizontal space for a fixed-width key column.
   * 'stacked' – key above value. Used in narrow contexts (e.g. the details card next
   *             to the graph) where a fixed key column would squeeze values into an
   *             unreadable sliver.
   */
  layout?: PropertiesLayout;
}

/**
 * Renders a node's `props` as a key-value list.
 * Complex values (objects / arrays, including JSON-serialized strings) are rendered
 * as collapsible JSON trees. This is the single source of truth for how a property
 * value is displayed — both the standalone Properties tab and the node details card
 * next to the graph use this component, just with a different `layout`.
 */
export default function PropertiesPanel({ props, title = 'Properties', layout = 'row' }: PropertiesPanelProps) {
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
          <PropertyRow key={key} name={key} value={props[key]} layout={layout} />
        ))}
      </div>
    </div>
  );
}

function PropertyRow({ name, value, layout }: { name: string; value: unknown; layout: PropertiesLayout }) {
  const [expanded, setExpanded] = useState(false);
  const isComplex = isComplexValue(value);
  const isUrl = !isComplex && isUrlValue(value);
  const parsedComplex = isComplex ? tryParseJson(value) : null;
  const stacked = layout === 'stacked';

  const keyEl = (
    <span
      className={
        stacked
          ? 'block font-mono text-[0.65rem] font-medium uppercase tracking-wide text-foreground-secondary dark:text-foreground-dark-secondary'
          : 'min-w-[160px] max-w-[200px] shrink-0 font-mono text-xs font-medium text-foreground-secondary dark:text-foreground-dark-secondary'
      }
    >
      {name}
    </span>
  );

  const valueEl = isComplex ? (
    <button
      onClick={() => setExpanded((v) => !v)}
      className="flex items-center gap-1 text-xs text-foreground-secondary hover:text-foreground dark:text-foreground-dark-secondary dark:hover:text-foreground-dark-default"
    >
      {expanded ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
      {expanded ? 'Collapse' : 'Expand'}
      <span className="ml-1 font-mono text-[0.6rem] opacity-60">
        {Array.isArray(parsedComplex)
          ? `[${(parsedComplex as unknown[]).length}]`
          : `{${Object.keys(parsedComplex as Record<string, unknown>).length}}`}
      </span>
    </button>
  ) : isUrl ? (
    <a
      href={String(value)}
      target="_blank"
      rel="noreferrer"
      className="break-all font-mono text-xs text-primary hover:underline"
    >
      {String(value)}
    </a>
  ) : (
    <span className="break-words font-mono text-xs text-foreground dark:text-foreground-dark-default">
      {String(value ?? '—')}
    </span>
  );

  return (
    <div className={`group relative px-4 py-2.5 text-sm hover:bg-gray-50 dark:hover:bg-white/5`}>
      {stacked ? (
        <div className="space-y-1 pr-6">
          {keyEl}
          <div>{valueEl}</div>
        </div>
      ) : (
        <div className="flex items-start gap-3">
          {keyEl}
          <div className="flex-1">{valueEl}</div>
        </div>
      )}

      {isComplex && expanded && (
        <pre
          className={`mt-2 overflow-x-auto rounded-md bg-gray-100 p-3 text-xs dark:bg-white/10 ${stacked ? '' : 'ml-[172px]'}`}
        >
          {JSON.stringify(parsedComplex, null, 2)}
        </pre>
      )}
    </div>
  );
}