import { useMemo } from 'react';
import { Info, ArrowUpRight, ArrowDownRight } from 'lucide-react';
import { NodeResource, nodeProps } from '../lib/catalogApi';
import { inferColumns, formatPropValue, parsePath, RelationSummary } from '../lib/kindUtils';

interface GenericTableProps {
  nodes: NodeResource[];
  kind: string;
  onSelect: (node: NodeResource) => void;
  relationSummaries?: Map<string, RelationSummary>;
}

/**
 * A generic, kind-agnostic table that:
 * 1. Infers columns from the union of all node props.
 * 2. Always shows `name` (short name from path) and `namespace` as first columns.
 * 3. When `relationSummaries` is provided, shows a "Relations" column
 *    with badges per relation kind (inbound/outbound counts).
 * 4. Renders a clickable "Details" action column last.
 */
export default function GenericTable({ nodes, kind, onSelect, relationSummaries }: GenericTableProps) {
  // Pre-compute parsed path data for all nodes
  const parsedPaths = useMemo(
    () => new Map(nodes.map((n) => [n.name, parsePath(n.path)])),
    [nodes]
  );
  const columns = useMemo(() => inferColumns(nodes), [nodes]);
  const hasRelations = relationSummaries !== undefined;
  const CORE_COL_COUNT = 2; // name + namespace
  const pluginColCount = columns.length - CORE_COL_COUNT;
  const hasPluginColumns = pluginColCount > 0;

  // Grid: prop columns + (optional relations) + actions
  const gridTemplateColumns = useMemo(() => {
    const propPart = columns.length <= 1
      ? 'minmax(140px, 1fr)'
      : `minmax(140px, 1fr) repeat(${columns.length - 1}, minmax(100px, 1fr))`;

    const relationsPart = hasRelations ? ' minmax(180px, 2fr)' : '';
    return `${propPart}${relationsPart} 80px`;
  }, [columns, hasRelations]);

  if (nodes.length === 0) {
    return (
      <p className="px-4 py-8 text-center text-sm text-foreground-secondary dark:text-foreground-dark-secondary">
        No {kind} nodes found.
      </p>
    );
  }

  return (
    <div className="overflow-x-auto rounded-lg border border-gray-200 dark:border-gray-700">
      {/* Header — two-row grid when plugin columns exist */}
      <div
        className="grid items-center gap-3 rounded-t-lg border-b border-gray-200 bg-gray-50 px-4 py-2 dark:border-gray-700 dark:bg-white/5"
        style={{
          gridTemplateColumns,
          ...(hasPluginColumns ? { gridTemplateRows: 'auto auto' } : {}),
        }}
      >
        {/* Row 1: Group headers (Core Metadata | Plugin Properties) */}
        {hasPluginColumns && (
          <>
            <div
              className="flex items-center text-[0.6rem] font-semibold uppercase tracking-wide text-foreground-secondary leading-none"
              style={{ gridColumn: `span ${CORE_COL_COUNT}`, gridRow: 1 }}
            >
              Core Metadata
            </div>
            <div
              className="flex items-center text-[0.6rem] font-semibold uppercase tracking-wide text-indigo-600 dark:text-indigo-400 leading-none rounded-[4px] bg-indigo-50/50 px-2 py-0.5 -mx-2 dark:bg-indigo-950/20"
              style={{ gridColumn: `span ${pluginColCount}`, gridRow: 1 }}
            >
              Plugin Properties
            </div>
            {/* Empty cells to fill remaining columns (Relations + Actions) */}
            {hasRelations && <div style={{ gridColumn: 'span 1', gridRow: 1 }} />}
            <div style={{ gridColumn: 'span 1', gridRow: 1 }} />
          </>
        )}

        {/* Row 2 / single row: Column header labels */}
        {columns.map((col, idx) => {
          const isPlugin = idx >= CORE_COL_COUNT;
          return (
            <span
              key={col}
              className={`truncate text-[0.65rem] font-semibold uppercase tracking-wide ${
                isPlugin
                  ? 'text-indigo-500/65 dark:text-indigo-400/55'
                  : 'text-foreground-secondary dark:text-foreground-dark-secondary'
              }`}
              style={{ gridRow: hasPluginColumns ? 2 : undefined }}
            >
              {col}
            </span>
          );
        })}
        {hasRelations && (
          <span
            className="text-[0.65rem] font-semibold uppercase tracking-wide text-foreground-secondary dark:text-foreground-dark-secondary"
            style={{ gridRow: hasPluginColumns ? 2 : undefined }}
          >
            Relations
          </span>
        )}
        <span
          className="text-[0.65rem] font-semibold uppercase tracking-wide text-foreground-secondary dark:text-foreground-dark-secondary"
          style={{ gridRow: hasPluginColumns ? 2 : undefined }}
        >
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
            const parsed = parsedPaths.get(node.name);

            if (col === 'name') {
              // Name column — short name from path
              return (
                <span
                  key={col}
                  className="truncate text-sm font-medium text-foreground dark:text-foreground-dark-default"
                  title={node.name}
                >
                  {parsed?.name ?? node.name}
                </span>
              );
            }

            if (col === 'namespace') {
              // Namespace column — second-to-last path segment
              return (
                <span
                  key={col}
                  className="truncate text-sm text-foreground-secondary dark:text-foreground-dark-secondary"
                  title={parsed?.namespace}
                >
                  {parsed?.namespace ?? '—'}
                </span>
              );
            }

            // Prop columns (from pluginClaims) — visual differentiation
            const props = nodeProps(node);
            const value = props[col];
            return (
              <span
                key={col}
                className="truncate text-sm italic text-foreground-secondary/75 dark:text-foreground-dark-secondary/70"
                title={typeof value === 'string' ? value : undefined}
              >
                {formatPropValue(value)}
              </span>
            );
          })}

          {/* Relations badges */}
          {hasRelations && (
            <RelationCell nodeName={node.name} summaries={relationSummaries!} />
          )}

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

/** Renders relation badges for a single node. */
function RelationCell({
  nodeName,
  summaries,
}: {
  nodeName: string;
  summaries: Map<string, RelationSummary>;
}) {
  const summary = summaries.get(nodeName);

  if (!summary) {
    return <span className="text-xs text-foreground-secondary dark:text-foreground-dark-secondary">—</span>;
  }

  const relationKinds = Object.keys(summary).sort();

  return (
    <div className="flex flex-wrap gap-1">
      {relationKinds.map((relKind) => {
        const { inbound, outbound } = summary[relKind];
        return (
          <span
            key={relKind}
            className="inline-flex items-center gap-1 rounded-md bg-gray-100 px-2 py-0.5 text-[0.65rem] font-medium text-foreground-secondary dark:bg-white/10 dark:text-foreground-dark-secondary"
            title={`${relKind}: ${inbound} inbound, ${outbound} outbound`}
          >
            {outbound > 0 && (
              <>
                <ArrowUpRight size={10} className="shrink-0 text-blue-500" />
                <span>{outbound}</span>
              </>
            )}
            {inbound > 0 && (
              <>
                <ArrowDownRight size={10} className="shrink-0 text-green-500" />
                <span>{inbound}</span>
              </>
            )}
            <span className="truncate max-w-[80px]">{relKind}</span>
          </span>
        );
      })}
    </div>
  );
}