import { useMemo } from 'react';
import { Info } from 'lucide-react';
import { NodeResource } from '../lib/catalogApi';
import { inferColumns, formatPropValue } from '../lib/kindUtils';

interface GenericTableProps {
  nodes: NodeResource[];
  kind: string;
  onSelect: (node: NodeResource) => void;
}

/**
 * A generic, kind-agnostic table that:
 * 1. Infers columns from the union of all node props.
 * 2. Always shows `name` as the first column.
 * 3. Renders a clickable "Details" action column last.
 */
export default function GenericTable({ nodes, kind, onSelect }: GenericTableProps) {
  const columns = useMemo(() => inferColumns(nodes), [nodes]);
  // Grid: name + each prop column + actions column
  const gridTemplateColumns = `minmax(140px, 1fr) repeat(${columns.length - 1}, minmax(100px, 1fr)) 80px`;

  if (nodes.length === 0) {
    return (
      <p className="px-4 py-8 text-center text-sm text-foreground-secondary dark:text-foreground-dark-secondary">
        No {kind} nodes found.
      </p>
    );
  }

  return (
    <div className="overflow-x-auto rounded-lg border border-gray-200 dark:border-gray-700">
      {/* Header */}
      <div
        className="grid items-center gap-3 rounded-t-lg border-b border-gray-200 bg-gray-50 px-4 py-2 dark:border-gray-700 dark:bg-white/5"
        style={{ gridTemplateColumns }}
      >
        {columns.map((col) => (
          <span
            key={col}
            className="truncate text-[0.65rem] font-semibold uppercase tracking-wide text-foreground-secondary dark:text-foreground-dark-secondary"
          >
            {col}
          </span>
        ))}
        <span className="text-[0.65rem] font-semibold uppercase tracking-wide text-foreground-secondary dark:text-foreground-dark-secondary">
          Actions
        </span>
      </div>

      {/* Rows */}
      {nodes.map((node) => (
        <div
          key={node.name}
          className="grid items-center gap-3 border-b border-gray-200 px-4 py-3 last:border-0 hover:bg-gray-50 dark:border-gray-700 dark:hover:bg-white/5"
          style={{ gridTemplateColumns }}
        >
          {columns.map((col, idx) => {
            if (idx === 0) {
              // Name column — always the first
              return (
                <span
                  key={col}
                  className="truncate text-sm font-medium text-foreground dark:text-foreground-dark-default"
                  title={node.name}
                >
                  {node.name}
                </span>
              );
            }
            // Prop columns
            const value = node.props?.[col];
            return (
              <span
                key={col}
                className="truncate text-sm text-foreground-secondary dark:text-foreground-dark-secondary"
                title={typeof value === 'string' ? value : undefined}
              >
                {formatPropValue(value)}
              </span>
            );
          })}

          {/* Actions */}
          <button
            onClick={() => onSelect(node)}
            className="flex items-center gap-1.5 rounded-md px-2 py-1 text-xs text-foreground-secondary hover:bg-gray-100 hover:text-foreground dark:text-foreground-dark-secondary dark:hover:bg-white/10 dark:hover:text-foreground-dark-default transition-colors"
            title="View details"
          >
            <Info size={14} />
            Details
          </button>
        </div>
      ))}
    </div>
  );
}
