import { useTable } from '@tanstack/react-table';
import {
  type ColumnDef,
  columnVisibilityFeature,
  createSortedRowModel,
  rowSortingFeature,
  type SortingState,
  tableFeatures,
} from '@tanstack/table-core';
import { ArrowDown, ArrowUp, ArrowUpDown } from 'lucide-react';
import { useMemo, useState } from 'react';
import { Badge } from '@/components/ui/badge';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import type { RadarEntry, RadarRing } from '@/lib/techRadar';
import { ringColor } from '@/lib/techRadar';
import MovementIndicator from './MovementIndicator';

interface RadarEntryTableProps {
  entries: RadarEntry[];
  rings: RadarRing[];
}

const features = tableFeatures({
  columnVisibilityFeature,
  rowSortingFeature,
  sortedRowModel: createSortedRowModel(),
});

/**
 * Tabular quadrant detail: one row per radar entry with ring, movement,
 * owner, and the recorded rationale.
 */
export default function RadarEntryTable({ entries, rings }: RadarEntryTableProps) {
  const labelText =
    'truncate text-[0.65rem] font-semibold uppercase tracking-wide text-muted-foreground';

  const ringIndexById = useMemo(
    () => new Map(rings.map((ring, index) => [ring.id, index])),
    [rings],
  );
  const ringNameById = useMemo(() => new Map(rings.map((ring) => [ring.id, ring.name])), [rings]);

  const columnDefs = useMemo<ColumnDef<typeof features, RadarEntry>[]>(
    () => [
      {
        id: 'entry',
        header: 'Entry',
        accessorFn: (entry) => entry.name,
        cell: (info) => (
          <span className="text-sm font-medium text-foreground">
            <span className="mr-1.5 text-xs text-muted-foreground">
              {info.row.original.index + 1}.
            </span>
            {info.row.original.name}
          </span>
        ),
      },
      {
        id: 'ring',
        header: 'Ring',
        accessorFn: (entry) => ringIndexById.get(entry.ring) ?? 0,
        cell: (info) => {
          const ring = info.row.original.ring;
          return (
            <Badge
              className="border-transparent text-white"
              style={{ backgroundColor: ringColor(ringIndexById.get(ring) ?? 0) }}
            >
              {ringNameById.get(ring) ?? ring}
            </Badge>
          );
        },
      },
      {
        id: 'movement',
        header: 'Movement',
        accessorFn: (entry) => entry.moved,
        cell: (info) =>
          info.row.original.moved === 'none' ? (
            <span className="text-xs text-muted-foreground">—</span>
          ) : (
            <MovementIndicator moved={info.row.original.moved} />
          ),
      },
      {
        id: 'owner',
        header: 'Owner',
        accessorFn: (entry) => entry.owner,
        cell: (info) => (
          <span className="text-sm text-muted-foreground">{info.row.original.owner}</span>
        ),
      },
      {
        id: 'rationale',
        header: 'Rationale',
        enableSorting: false,
        accessorFn: (entry) => entry.rationale,
        cell: (info) => (
          <span className="block max-w-xl whitespace-pre-line break-words text-sm text-muted-foreground">
            {info.row.original.rationale}
          </span>
        ),
      },
    ],
    [ringIndexById, ringNameById],
  );

  const [sorting, setSorting] = useState<SortingState>([]);

  const table = useTable({
    features,
    data: entries,
    columns: columnDefs,
    state: { sorting },
    onSortingChange: setSorting,
  });

  return (
    <div className="overflow-x-auto rounded-lg border border-gray-200">
      <Table>
        <TableHeader>
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
              <TableCell colSpan={columnDefs.length} className="text-center text-muted-foreground">
                No entries match the active filters.
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </div>
  );
}
