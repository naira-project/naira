import { useState, type ReactNode } from 'react';
import { useParams, useNavigate } from 'react-router';
import Markdown from 'react-markdown';
import { ExternalLink, GitBranch } from 'lucide-react';
import { useDatasetWithId, type Dataset } from '../hooks/useDatasets';
import { Badge } from '../components/ui/badge';
import { cn } from '../lib/utils';
import ColumnTable from '../components/ColumnTable';
import CatalogGraph from './CatalogGraph';

function MetaField({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div>
      <p className="text-[0.65rem] font-semibold uppercase tracking-wide text-foreground-secondary dark:text-foreground-dark-secondary">
        {label}
      </p>
      <div className="mt-1 text-sm text-foreground dark:text-foreground-dark-default">{children}</div>
    </div>
  );
}

function Overview({ dataset }: { dataset: Dataset }) {
  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-3">
        <MetaField label="Platform">{dataset.platform || '—'}</MetaField>
        <MetaField label="Owners">{dataset.owners.join(', ') || '—'}</MetaField>
        <MetaField label="Source">{dataset.source || '—'}</MetaField>
        <MetaField label="FQN">
          <span className="break-words font-mono text-xs">{dataset.fqn || '—'}</span>
        </MetaField>
        <MetaField label="Columns">{dataset.columns.length || '—'}</MetaField>
        <MetaField label="Lineage">
          <span className="inline-flex items-center gap-1">
            <GitBranch size={13} className="shrink-0" />
            {dataset.derivedFrom}↑ {dataset.feeds}↓
          </span>
        </MetaField>
      </div>

      <MetaField label="Tags">
        {dataset.tags.length === 0 ? (
          '—'
        ) : (
          <span className="flex flex-wrap gap-1">
            {dataset.tags.map((tag) => (
              <Badge key={tag} variant="secondary" className="text-[0.65rem]">{tag}</Badge>
            ))}
          </span>
        )}
      </MetaField>

      {dataset.sourceUrl && (
        <MetaField label="External">
          <a
            href={dataset.sourceUrl}
            target="_blank"
            rel="noreferrer"
            className="inline-flex items-center gap-1 text-sm text-primary hover:underline"
          >
            Open in source <ExternalLink size={13} />
          </a>
        </MetaField>
      )}

      {dataset.description && (
        <div>
          <p className="mb-1 text-[0.65rem] font-semibold uppercase tracking-wide text-foreground-secondary dark:text-foreground-dark-secondary">
            Description
          </p>
          <div className="prose prose-sm max-w-none prose-headings:text-foreground prose-p:text-foreground prose-headings:mb-1 prose-headings:mt-4 prose-p:my-1 prose-ul:my-1 prose-li:my-0 dark:prose-invert">
            <Markdown>{dataset.description}</Markdown>
          </div>
        </div>
      )}
    </div>
  );
}

export default function DatasetSpec() {
  const navigate = useNavigate();
  const { id = '' } = useParams();
  const { dataset, loading, error } = useDatasetWithId(decodeURIComponent(decodeURIComponent(id)));
  const [tab, setTab] = useState<'Overview' | 'Columns' | 'Lineage'>('Overview');

  const tabs = ['Overview', 'Columns', 'Lineage'] as const;

  return (
    <div className="flex h-screen flex-col overflow-hidden bg-background dark:bg-background-dark-default">
      {/* Header */}
      <header className="flex shrink-0 flex-col gap-2 border-b border-gray-200 px-6 py-3 dark:border-gray-700">
        <button
          onClick={() => navigate('/dataset-registry')}
          className="self-start text-sm text-foreground-secondary transition-colors hover:text-foreground dark:text-foreground-dark-secondary dark:hover:text-foreground-dark-default"
        >
          ← Dataset Registry
        </button>
        <h1 className="truncate text-2xl font-medium text-foreground dark:text-foreground-dark-default" title={dataset?.fqn || dataset?.name}>
          {dataset?.name ?? 'Dataset'}
        </h1>

        {dataset && (
          <div className="flex gap-1">
            {tabs.map((value) => (
              <button
                key={value}
                onClick={() => setTab(value)}
                className={cn(
                  'cursor-pointer rounded-md px-3 py-1.5 text-sm transition-colors',
                  tab === value
                    ? 'bg-primary font-semibold text-primary-foreground'
                    : 'bg-accent text-foreground hover:bg-accent/70'
                )}
              >
                {value}
              </button>
            ))}
          </div>
        )}
      </header>

      {/* Content */}
      <div className="flex-1 overflow-y-auto px-6 py-4">
        {loading && <p className="text-sm text-foreground-secondary dark:text-foreground-dark-secondary">Loading…</p>}
        {error && <p className="text-sm text-red-500">{error}</p>}

        {!loading && !error && dataset && (
          <>
            {tab === 'Overview' && <Overview dataset={dataset} />}

            {tab === 'Columns' && (
              dataset.columns.length === 0
                ? <p className="text-sm text-foreground-secondary dark:text-foreground-dark-secondary">No columns available for this dataset.</p>
                : <ColumnTable dataset={dataset} />
            )}

            {tab === 'Lineage' && (
              <CatalogGraph rootNode={{ name: dataset.resourceName, label: dataset.name }} />
            )}
          </>
        )}
      </div>
    </div>
  );
}
