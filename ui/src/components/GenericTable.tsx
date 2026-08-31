import { useTable } from '@tanstack/react-table';
import {
  type ColumnDef,
  type ColumnVisibilityState,
  columnFilteringFeature,
  columnVisibilityFeature,
  createFilteredRowModel,
  createSortedRowModel,
  globalFilteringFeature,
  rowSortingFeature,
  type SortingState,
  tableFeatures,
} from '@tanstack/table-core';
import {
  ArrowDown,
  ArrowDownRight,
  ArrowUp,
  ArrowUpDown,
  ArrowUpRight,
  Columns3,
  Info,
  Search,
} from 'lucide-react';
import { useMemo, useState } from 'react';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Input } from '@/components/ui/input';

import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  formatPropValue,
  inferColumns,
  namespaceColumnLabel,
  parsePath,
  type RelationSummary,
} from '@/lib/kindUtils';
import { type NodeResource, nodeProps } from '../lib/catalogApi';
import EmptyState from './states/EmptyState';

interface GenericTableProps {
  nodes: NodeResource[];
  kind: string;
  onSelect: (node: NodeResource) => void;
  relationSummaries: Map<string, RelationSummary>;
  columns?: string[];
}

// Only the features we actually use are pulled in, keeping the table tree-shakeable.
const features = tableFeatures({
  columnFilteringFeature,
  columnVisibilityFeature,
  globalFilteringFeature,
  rowSortingFeature,
  filteredRowModel: createFilteredRowModel(),
  sortedRowModel: createSortedRowModel(),
});

export default function GenericTable({
  nodes,
  kind,
  onSelect,
  relationSummaries,
  columns,
}: GenericTableProps) {
  const labelText =
    'truncate text-[0.65rem] font-semibold uppercase tracking-wide text-muted-foreground';
  const groupText =
    'flex items-center text-[0.6rem] font-bold uppercase tracking-wider text-muted-foreground/70 leading-none';
  const [columnVisibility, setColumnVisibility] = useState<ColumnVisibilityState>({});

  const parsedPaths = useMemo(
    () => new Map(nodes.map((n) => [n.name, parsePath(n.path)])),
    [nodes],
  );

  // inferColumns returns ['name', 'namespace', ...pluginProps]; name/namespace/relations are
  // rendered explicitly as core columns below, so only the plugin tail is needed here.
  const pluginColumns = useMemo(() => {
    const inferred = inferColumns(nodes).slice(2);
    return columns ? columns.filter((col) => inferred.includes(col)) : inferred;
  }, [nodes, columns]);
  const pluginColCount = pluginColumns.length;
  const namespaceLabel = namespaceColumnLabel(kind);
  const hasPluginColumns = pluginColCount > 0;
  const CORE_COL_COUNT = 3; // name + namespace + relations

  const columnDefs = useMemo<ColumnDef<typeof features, NodeResource>[]>(
    () => [
      {
        id: 'name',
        header: 'Name',
        accessorFn: (node) => parsedPaths.get(node.name)?.name ?? node.name,
        cell: (info) => (
          <span
            className="truncate text-sm font-medium text-foreground"
            title={info.row.original.name}
          >
            {info.getValue() as string}
          </span>
        ),
      },
      {
        id: namespaceLabel,
        header: namespaceLabel.charAt(0).toUpperCase() + namespaceLabel.slice(1),
        accessorFn: (node) => parsedPaths.get(node.name)?.namespace ?? '—',
        cell: (info) => (
          <span
            className="truncate text-sm text-muted-foreground"
            title={parsedPaths.get(info.row.original.name)?.namespace}
          >
            {info.getValue() as string}
          </span>
        ),
      },
      {
        id: 'relations',
        header: 'Relations',
        enableSorting: false,
        cell: (info) => (
          <RelationCell nodeName={info.row.original.name} summaries={relationSummaries} />
        ),
      },
      ...pluginColumns.map(
        (col): ColumnDef<typeof features, NodeResource> => ({
          id: col,
          header: col,
          accessorFn: (node) => nodeProps(node)[col],
          cell: (info) => {
            const value = info.getValue();
            return (
              <span
                className="truncate text-sm italic text-muted-foreground/75"
                title={typeof value === 'string' ? value : undefined}
              >
                {formatPropValue(value)}
              </span>
            );
          },
        }),
      ),
      {
        id: 'actions',
        header: 'Actions',
        enableSorting: false,
        cell: (info) => (
          <Button
            variant="ghost"
            size="xs"
            onClick={() => onSelect(info.row.original)}
            title="View details"
          >
            <Info size={14} />
            Details
          </Button>
        ),
      },
    ],
    [namespaceLabel, parsedPaths, pluginColumns, relationSummaries, onSelect],
  );

  const [sorting, setSorting] = useState<SortingState>([]);
  const [globalFilter, setGlobalFilter] = useState('');

  const table = useTable({
    features,
    data: nodes,
    columns: columnDefs,
    state: { sorting, globalFilter, columnVisibility },
    onSortingChange: setSorting,
    onGlobalFilterChange: setGlobalFilter,
    onColumnVisibilityChange: setColumnVisibility,
  });

  if (nodes.length === 0) {
    return <EmptyState />;
  }

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-row gap-3">
        <Input
          value={globalFilter}
          onChange={(event) => setGlobalFilter(event.target.value)}
          placeholder="Filter nodes…"
          startAdornment={<Search className="size-4" />}
          className="max-w-sm"
        />

        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="outline" size="sm">
              <Columns3 className="size-4" />
              Columns
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuLabel>Toggle columns</DropdownMenuLabel>
            <DropdownMenuSeparator />
            {table.getAllLeafColumns().map((column) => (
              <DropdownMenuCheckboxItem
                key={column.id}
                checked={column.getIsVisible()}
                onCheckedChange={(checked) => column.toggleVisibility(!!checked)}
                onSelect={(e) => e.preventDefault()} // keep menu open after each click
              >
                {column.id}
              </DropdownMenuCheckboxItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      <div className="overflow-x-auto rounded-lg border border-gray-200">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead colSpan={CORE_COL_COUNT} className="bg-gray-50">
                <span className={groupText}>Core Metadata</span>
              </TableHead>
              {hasPluginColumns && (
                <TableHead colSpan={pluginColCount} className="bg-gray-50">
                  <span className={groupText}>Plugin Properties</span>
                </TableHead>
              )}
              <TableHead className="bg-gray-50" />{' '}
              {/* spacer over the Actions column */}
            </TableRow>

            {table.getHeaderGroups().map((headerGroup) => (
              <TableRow key={headerGroup.id}>
                {headerGroup.headers.map((header) => {
                  const sortDirection = header.column.getIsSorted();
                  return (
                    <TableHead
                      key={header.id}
                      className={
                        header.column.getCanSort() ? 'cursor-pointer select-none' : undefined
                      }
                      onClick={header.column.getToggleSortingHandler()}
                    >
                      <div className="flex items-center gap-1">
                        <span className={labelText}>
                          <table.FlexRender header={header} />
                        </span>
                        {header.column.getCanSort() &&
                          (sortDirection === 'asc' ? (
                            <ArrowUp className="size-3.5" />
                          ) : sortDirection === 'desc' ? (
                            <ArrowDown className="size-3.5" />
                          ) : (
                            <ArrowUpDown className="size-3.5 text-muted-foreground" />
                          ))}
                      </div>
                    </TableHead>
                  );
                })}
              </TableRow>
            ))}
          </TableHeader>
          <TableBody>
            {table.getRowModel().rows.length ? (
              table.getRowModel().rows.map((row) => (
                <TableRow key={row.id}>
                  {row.getVisibleCells().map((cell) => (
                    <TableCell key={cell.id}>
                      <table.FlexRender cell={cell} />
                    </TableCell>
                  ))}
                </TableRow>
              ))
            ) : (
              <TableRow>
                <TableCell
                  colSpan={columnDefs.length}
                  className="text-center text-muted-foreground"
                >
                  No nodes found.
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
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
    return <span className="text-xs text-muted-foreground">—</span>;
  }

  const relationKinds = Object.keys(summary).sort();

  return (
    <div className="flex flex-wrap gap-1">
      {relationKinds.map((relKind) => {
        const { inbound, outbound } = summary[relKind];
        return (
          <span
            key={relKind}
            className="inline-flex items-center gap-1 rounded-md bg-gray-100 px-2 py-0.5 text-[0.65rem] font-medium text-muted-foreground"
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
