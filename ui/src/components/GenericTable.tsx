import { useMemo } from 'react';
import { Info, ArrowUpRight, ArrowDownRight } from 'lucide-react';
import { NodeResource, nodeProps } from '../lib/catalogApi';
import { inferColumns, formatPropValue, parsePath,  namespaceColumnLabel, RelationSummary } from '../lib/kindUtils';
import EmptyState from './EmptyState';

interface GenericTableProps {
  nodes: NodeResource[];
  kind: string;
  onSelect: (node: NodeResource) => void;
  relationSummaries: Map<string, RelationSummary>;
  columns?: string[];
}

/**
 * A generic, kind-agnostic table that:
 * 1. Shows the plugin property columns a viewpoint pins, or every property found
 *    across the nodes when it pins none.
 * 2. Always shows `name`, `namespace`, and `Relations` as the "Core Metadata" columns.
 * 3. Renders plugin-derived prop columns as "Plugin Properties", immediately after Core Metadata.
 *    The group-header row (Core Metadata / Plugin Properties) is always rendered, even when a
 *    kind has zero plugin properties — this keeps table height/layout stable when switching
 *    between kinds instead of the header jumping around.
 * 4. Renders a clickable "Details" action column pinned to the right; every other column
 *    hugs the left edge (a flexible spacer column absorbs leftover space, and also stands in
 *    for "Plugin Properties" visually when there are no plugin columns).
 *
 * Implementation note: the whole table (header rows + every data row) lives inside a SINGLE
 * CSS grid container. Row wrappers use `display: contents` so their cells become direct grid
 * items. This matters — if each row were its own independent grid, `max-content` column
 * sizing would be computed separately per row and columns would drift out of alignment on
 * wider screens.
 */
export default function GenericTable({
  nodes,
  kind,
  onSelect,
  relationSummaries,
  columns: configuredColumns,
}: GenericTableProps) {
  const parsedPaths = useMemo(
    () => new Map(nodes.map((n) => [n.name, parsePath(n.path)])),
    [nodes]
  );

  // inferColumns returns ['name', 'namespace', ...pluginProps]; name/namespace/relations are
  // rendered explicitly as the core columns, so we only need the plugin tail here.
  const columns = useMemo(() => inferColumns(nodes), [nodes]);
  const pluginColumns = useMemo(
    () => configuredColumns ?? columns.slice(2),
    [configuredColumns?.join(','), columns]
  );
  const pluginColCount = pluginColumns.length;
  const namespaceLabel = namespaceColumnLabel(kind);
  const hasPluginColumns = pluginColCount > 0;
  const CORE_COL_COUNT = 3; // name + namespace + relations
  const ACTIONS_COL_WIDTH = '110px';

  // Core + plugin columns hug the left (content-sized), a flexible spacer absorbs leftover
  // width (and doubles as the "Plugin Properties" area when there are no plugin columns), and
  // Actions is pinned to the right.
  const gridTemplateColumns = useMemo(() => {
    const pluginPart = hasPluginColumns
      ? ` repeat(${pluginColCount}, minmax(100px, max-content))`
      : '';
    return `minmax(140px, max-content) minmax(100px, max-content) minmax(160px, max-content)${pluginPart} 1fr ${ACTIONS_COL_WIDTH}`;
  }, [pluginColCount, hasPluginColumns]);

  if (nodes.length === 0) {
    return <EmptyState />;
  }

  const headerCell =
    'flex items-center h-full bg-gray-50 px-4 py-2.5 dark:bg-white/5';
  const labelText =
    'truncate text-[0.65rem] font-semibold uppercase tracking-wide text-foreground-secondary dark:text-foreground-dark-secondary';
  const groupText =
    'flex items-center text-[0.6rem] font-bold uppercase tracking-wider text-foreground-secondary/70 leading-none';

  return (
    <div className="overflow-x-auto rounded-lg border border-gray-200 dark:border-gray-700">
      <div className="grid" style={{ gridTemplateColumns }}>
        {/* Row 1: group headers — always rendered so the table doesn't jump in height when a
            kind has no plugin properties. "Plugin Properties" spans the plugin columns plus the
            spacer, so it still shows even with zero plugin columns. */}
        <div
          className={`${headerCell} ${groupText} rounded-tl-lg`}
          style={{ gridColumn: `span ${CORE_COL_COUNT}` }}
        >
          Core Metadata
        </div>
        <div
          className={`${headerCell} ${groupText}`}
          style={{ gridColumn: `span ${pluginColCount + 1}` }}
        >
          Plugin Properties
        </div>
        <div className={`${headerCell} rounded-tr-lg`} />

        {/* Row 2: column labels */}
        <div className={`${headerCell} border-b border-gray-200 dark:border-gray-700`}>
          <span className={labelText}>name</span>
        </div>
        <div className={`${headerCell} border-b border-gray-200 dark:border-gray-700`}>
           <span className={labelText}>{namespaceLabel}</span>
        </div>
        <div className={`${headerCell} border-b border-gray-200 dark:border-gray-700`}>
          <span className={labelText}>Relations</span>
        </div>
        {pluginColumns.map((col) => (
          <div
            key={col}
            className={`${headerCell} border-b border-gray-200 dark:border-gray-700`}
          >
            <span className={labelText}>{col}</span>
          </div>
        ))}
        <div
          className={`${headerCell} border-b border-gray-200 dark:border-gray-700`}
        />
        <div className={`${headerCell} border-b border-gray-200 dark:border-gray-700`}>
          <span className={labelText}>Actions</span>
        </div>

        {/* Data rows — `contents` makes each row's cells direct grid items so they share the
            same column tracks as the header (see note above). */}
        {nodes.map((node, rowIdx) => {
          const parsed = parsedPaths.get(node.name);
          const props = nodeProps(node);
          const isLastRow = rowIdx === nodes.length - 1;
          const cellBase = `flex items-center h-full px-4 py-3 group-hover/row:bg-gray-50 dark:group-hover/row:bg-white/5 ${
            isLastRow ? '' : 'border-b border-gray-200 dark:border-gray-700'
          }`;

          return (
            <div key={node.name} className="contents group/row">
              <div className={`${cellBase} truncate ${isLastRow ? 'rounded-bl-lg' : ''}`}>
                <span
                  className="truncate text-sm font-medium text-foreground dark:text-foreground-dark-default"
                  title={node.name}
                >
                  {parsed?.name ?? node.name}
                </span>
              </div>

              <div className={`${cellBase} truncate`}>
                <span
                  className="truncate text-sm text-foreground-secondary dark:text-foreground-dark-secondary"
                  title={parsed?.namespace}
                >
                  {parsed?.namespace ?? '—'}
                </span>
              </div>

              <div className={cellBase}>
                <RelationCell nodeName={node.name} summaries={relationSummaries} />
              </div>

              {pluginColumns.map((col) => {
                const value = props[col];
                return (
                  <div
                    key={col}
                    className={`${cellBase}`}
                  >
                    <span
                      className="truncate text-sm italic text-foreground-secondary/75 dark:text-foreground-dark-secondary/70"
                      title={typeof value === 'string' ? value : undefined}
                    >
                      {formatPropValue(value)}
                    </span>
                  </div>
                );
              })}

              {/* Spacer — absorbs leftover width so only Actions sits on the right */}
              <div
                className={`${cellBase}`}
              />

              <div className={`${cellBase} ${isLastRow ? 'rounded-br-lg' : ''}`}>
                <button
                  onClick={() => onSelect(node)}
                  className="flex items-center gap-1.5 rounded-md px-2 py-1 text-xs text-foreground-secondary hover:bg-gray-100 hover:text-foreground dark:text-foreground-dark-secondary dark:hover:bg-white/10 dark:hover:text-foreground-dark-default transition-colors"
                  title="View details"
                >
                  <Info size={14} />
                  Details
                </button>
              </div>
            </div>
          );
        })}
      </div>
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