import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router';
import { Search, ChevronDown, ChevronRight, ExternalLink, Database, GitBranch } from 'lucide-react';
import { Input } from '../components/ui/input';
import { Badge } from '../components/ui/badge';
import { cn } from '../lib/utils';
import ColumnTable from '../components/ColumnTable';
import { useDatasets, type Dataset } from '../hooks/useDatasets';

const COL_WIDTHS = '24% 14% 18% 14% 9% 11% 10%';
const HEADERS = ['Dataset', 'Platform', 'Tags', 'Owners', 'Columns', 'Lineage', 'Source'];

const SOURCE_LABELS: Record<string, string> = {
  openmetadata: 'OpenMetadata',
};

function sourceLabel(source: string) {
  return SOURCE_LABELS[source] ?? source;
}

function TableHeader() {
  return (
    <div
      className="grid rounded-t-[6px] border-b border-gray-200 bg-gray-50 px-4 py-2 dark:border-gray-700 dark:bg-white/5"
      style={{ gridTemplateColumns: COL_WIDTHS }}
    >
      {HEADERS.map((header) => (
        <span key={header} className="text-[0.65rem] font-semibold uppercase tracking-wide text-foreground-secondary dark:text-foreground-dark-secondary">
          {header}
        </span>
      ))}
    </div>
  );
}

function DatasetRow({ dataset }: { dataset: Dataset }) {
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const hasColumns = dataset.columns.length > 0;
  const hasLineage = dataset.derivedFrom > 0 || dataset.feeds > 0;

  return (
    <div className="border-b border-gray-200 last:border-0 dark:border-gray-700">
      <div
        className="grid cursor-pointer items-center px-4 py-3 hover:bg-gray-50 dark:hover:bg-white/5"
        style={{ gridTemplateColumns: COL_WIDTHS }}
        onClick={() => setOpen((value) => !value)}
      >
        <span
          className="flex items-center gap-1.5 truncate text-sm font-medium text-foreground dark:text-foreground-dark-default"
          title={dataset.fqn || dataset.name}
        >
          {hasColumns
            ? (open ? <ChevronDown size={14} className="shrink-0" /> : <ChevronRight size={14} className="shrink-0" />)
            : <span className="w-[14px] shrink-0" />}
          <button
            onClick={(event) => {
              event.stopPropagation();
              navigate(`/dataset-registry/${encodeURIComponent(encodeURIComponent(dataset.id))}`);
            }}
            className="truncate text-left hover:text-primary hover:underline"
            title="View details"
          >
            {dataset.name}
          </button>
        </span>
        <span className="truncate text-sm text-foreground-secondary dark:text-foreground-dark-secondary">{dataset.platform || '—'}</span>
        <span className="flex flex-wrap gap-1">
          {dataset.tags.length === 0
            ? <span className="text-sm text-foreground-secondary dark:text-foreground-dark-secondary">—</span>
            : dataset.tags.map((tag) => <Badge key={tag} variant="secondary" className="text-[0.65rem]">{tag}</Badge>)}
        </span>
        <span className="truncate text-sm text-foreground-secondary dark:text-foreground-dark-secondary" title={dataset.owners.join(', ')}>
          {dataset.owners.join(', ') || '—'}
        </span>
        <span className="text-sm text-foreground-secondary dark:text-foreground-dark-secondary">{dataset.columns.length || '—'}</span>
        <span className="text-sm text-foreground-secondary dark:text-foreground-dark-secondary">
          {hasLineage ? (
            <span className="inline-flex items-center gap-1" title={`Derived from ${dataset.derivedFrom} · feeds ${dataset.feeds}`}>
              <GitBranch size={13} className="shrink-0" />
              {dataset.derivedFrom}↑ {dataset.feeds}↓
            </span>
          ) : '—'}
        </span>
        <span>
          {dataset.sourceUrl ? (
            <a
              href={dataset.sourceUrl}
              target="_blank"
              rel="noreferrer"
              onClick={(event) => event.stopPropagation()}
              className="inline-flex items-center gap-1 text-xs text-primary hover:underline"
            >
              Open <ExternalLink size={12} />
            </a>
          ) : <span className="text-sm text-foreground-secondary dark:text-foreground-dark-secondary">—</span>}
        </span>
      </div>

      {open && hasColumns && (
        <div className="bg-gray-50/60 px-10 py-3 dark:bg-white/5">
          {dataset.description && (
            <p className="mb-2 text-xs text-foreground-secondary dark:text-foreground-dark-secondary">{dataset.description}</p>
          )}
          <ColumnTable dataset={dataset} />
        </div>
      )}
    </div>
  );
}

export default function Datasets() {
  const [search, setSearch] = useState('');
  const [source, setSource] = useState<string>('all');
  const { datasets, loading, error } = useDatasets();

  const sources = useMemo(() => {
    const unique = new Set<string>();
    datasets.forEach((dataset) => {
      if (dataset.source) {
        unique.add(dataset.source);
      }
    });
    return Array.from(unique).sort();
  }, [datasets]);

  const tabs = useMemo(
    () => [{ value: 'all', label: 'All' }, ...sources.map((value) => ({ value, label: sourceLabel(value) }))],
    [sources]
  );

  const filtered = useMemo(() => {
    const query = search.trim().toLowerCase();
    return datasets.filter((dataset) => {
      if (source !== 'all' && dataset.source !== source) {
        return false;
      }
      if (!query) {
        return true;
      }
      return (
        dataset.name.toLowerCase().includes(query) ||
        dataset.fqn.toLowerCase().includes(query) ||
        dataset.platform.toLowerCase().includes(query) ||
        dataset.tags.some((tag) => tag.toLowerCase().includes(query))
      );
    });
  }, [datasets, search, source]);

  return (
    <div className="flex h-screen overflow-hidden bg-background dark:bg-background-dark-default">
      <div className="flex flex-1 flex-col overflow-hidden">

        {/* Top bar */}
        <header className="flex shrink-0 items-center gap-3 border-b border-gray-200 bg-white px-6 py-3 dark:border-gray-700 dark:bg-background-dark-paper">
          <Database size={18} className="text-primary" />
          <h1 className="text-base font-semibold text-foreground dark:text-foreground-dark-default">Dataset Registry</h1>
          <Input
            startAdornment={<Search size={16} />}
            placeholder="Search datasets by name, FQN, platform or tag..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="max-w-[480px]"
          />
        </header>

        <div className="flex-1 overflow-y-auto px-6 py-4">

          {/* Source tabs */}
          <div className="mb-4 flex flex-wrap gap-2">
            {tabs.map(({ value, label }) => (
              <button
                key={value}
                onClick={() => setSource(value)}
                className={cn(
                  'rounded-lg px-4 py-1.5 text-sm transition-colors',
                  source === value
                    ? 'bg-primary font-semibold text-white'
                    : 'bg-gray-100 font-normal text-foreground hover:bg-gray-200 dark:bg-white/10 dark:text-foreground-dark-default dark:hover:bg-white/20'
                )}
              >
                {label}
              </button>
            ))}
          </div>

          {loading && <p className="text-sm text-foreground-secondary dark:text-foreground-dark-secondary">Loading…</p>}
          {error && <p className="text-sm text-red-500">{error}</p>}

          {!loading && !error && (
            <>
              <p className="mb-3 text-xs text-foreground-secondary dark:text-foreground-dark-secondary">
                {filtered.length} dataset{filtered.length === 1 ? '' : 's'}
              </p>
              <div className="overflow-hidden rounded-[6px] border border-gray-200 dark:border-gray-700">
                <TableHeader />
                {filtered.length === 0 ? (
                  <p className="px-4 py-3 text-sm text-foreground-secondary dark:text-foreground-dark-secondary">
                    No datasets found. Trigger an ingestion (POST /v1/plugins:run) to populate the catalog.
                  </p>
                ) : (
                  filtered.map((dataset) => <DatasetRow key={dataset.id} dataset={dataset} />)
                )}
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
