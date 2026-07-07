import { type Dataset } from '../hooks/useDatasets';

const GRID = '30% 25% 45%';

export default function ColumnTable({ dataset }: { dataset: Dataset }) {
  return (
    <div className="overflow-hidden rounded-md border border-gray-200 dark:border-gray-700">
      <div
        className="grid border-b border-gray-200 bg-gray-100 px-3 py-1.5 dark:border-gray-700 dark:bg-white/10"
        style={{ gridTemplateColumns: GRID }}
      >
        {['Column', 'Type', 'Description'].map((header) => (
          <span key={header} className="text-[0.6rem] font-semibold uppercase tracking-wide text-foreground-secondary dark:text-foreground-dark-secondary">
            {header}
          </span>
        ))}
      </div>
      {dataset.columns.map((column) => (
        <div
          key={column.name}
          className="grid border-b border-gray-100 px-3 py-1.5 last:border-0 dark:border-gray-700/60"
          style={{ gridTemplateColumns: GRID }}
        >
          <span className="truncate text-xs font-medium text-foreground dark:text-foreground-dark-default">{column.name}</span>
          <span className="truncate text-xs text-foreground-secondary dark:text-foreground-dark-secondary" title={column.nativeType || column.type}>
            {column.type || '—'}
          </span>
          <span className="truncate text-xs text-foreground-secondary dark:text-foreground-dark-secondary">{column.description || '—'}</span>
        </div>
      ))}
    </div>
  );
}
