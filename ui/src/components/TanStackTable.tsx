import { useMemo, useState } from "react";
import {
  ColumnDef,
  columnFilteringFeature,
  columnVisibilityFeature,
  createFilteredRowModel,
  createSortedRowModel,
  filterFns,
  globalFilteringFeature,
  rowSortingFeature,
  SortingState,
  sortFns,
  tableFeatures,
} from "@tanstack/table-core";
import { useTable } from "@tanstack/react-table";
import { ArrowDown, ArrowUp, ArrowUpDown, Search } from "lucide-react";

import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Input } from "@/components/ui/input";
import { nodeProps, NodeResource } from "../lib/catalogApi";
import { useCatalogNodes } from "../hooks/useCatalogNodes";

interface TanStackTableProps {
  kind: string;
}

// Only the features we actually use are pulled in, keeping the table tree-shakeable.
const features = tableFeatures({
  columnFilteringFeature,
  columnVisibilityFeature,
  globalFilteringFeature,
  rowSortingFeature,
  filteredRowModel: createFilteredRowModel(),
  sortedRowModel: createSortedRowModel(),
  filterFns,
  sortFns,
});

const columns: ColumnDef<typeof features, NodeResource>[] = [
  {
    accessorKey: "name",
    header: "Name",
  },
  {
    accessorKey: "kind",
    header: "Kind",
  },
  {
    accessorKey: "path",
    header: "Path",
  },
  {
    id: "plugins",
    header: "Plugins",
    accessorFn: (node) => node.pluginClaims.map((claim) => claim.plugin).join(", "),
  },
  {
    id: "props",
    header: "Props",
    accessorFn: (node) => Object.keys(nodeProps(node)).length,
    sortFn: "alphanumeric",
  },
];

export default function TanStackTable({ kind }: TanStackTableProps) {
  const { nodes, loading, error } = useCatalogNodes(kind);
  const [sorting, setSorting] = useState<SortingState>([]);
  const [globalFilter, setGlobalFilter] = useState("");

  const data = useMemo(() => nodes, [nodes]);

  const table = useTable({
    features,
    data,
    columns,
    state: { sorting, globalFilter },
    onSortingChange: setSorting,
    onGlobalFilterChange: setGlobalFilter,
  });

  if (loading) {
    return <p className="text-sm text-muted-foreground">Loading nodes…</p>;
  }

  if (error) {
    return <p className="text-sm text-destructive">{error}</p>;
  }

  return (
    <div className="flex flex-col gap-3">
      <Input
        value={globalFilter}
        onChange={(event) => setGlobalFilter(event.target.value)}
        placeholder="Filter nodes…"
        startAdornment={<Search className="size-4" />}
        className="max-w-sm"
      />

      <Table>
        <TableHeader>
          {table.getHeaderGroups().map((headerGroup) => (
            <TableRow key={headerGroup.id}>
              {headerGroup.headers.map((header) => {
                const sortDirection = header.column.getIsSorted();
                return (
                  <TableHead
                    key={header.id}
                    className={header.column.getCanSort() ? "cursor-pointer select-none" : undefined}
                    onClick={header.column.getToggleSortingHandler()}
                  >
                    <div className="flex items-center gap-1">
                      <table.FlexRender header={header} />
                      {header.column.getCanSort() &&
                        (sortDirection === "asc" ? (
                          <ArrowUp className="size-3.5" />
                        ) : sortDirection === "desc" ? (
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
              <TableCell colSpan={columns.length} className="text-center text-muted-foreground">
                No nodes found.
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </div>
  );
}