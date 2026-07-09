import { useState } from 'react';
import { useParams, useNavigate } from 'react-router';
import { ArrowLeft } from 'lucide-react';
import { useCatalogDetail } from '../hooks/useCatalogDetail';
import { nodeProps } from '../lib/catalogApi';
import PropertiesPanel from '../components/PropertiesPanel';
import CatalogGraph from './CatalogGraph';
import { cn } from '../lib/utils';

type DetailTab = 'Properties' | 'Graph';

/**
 * Generic detail page for any catalog node.
 * Displays:
 * - Node identity (name, kind, path)
 * - Graph tab: relationship graph centred on this node
 * - Properties tab: key-value rendering of all props
 */
export default function CatalogDetail() {
  const { kind = '', '*': path = '' } = useParams();
  const navigate = useNavigate();
  const decodedKind = decodeURIComponent(kind);
  const decodedPath = decodeURIComponent(path);
  const { node, loading, error } = useCatalogDetail(decodedKind, decodedPath);
  const [activeTab, setActiveTab] = useState<DetailTab>('Graph');

  const tabs: { value: DetailTab; label: string }[] = [
    { value: 'Graph', label: 'Graph' },
    { value: 'Properties', label: 'Properties' },
  ];

  return (
    <div className="flex h-screen overflow-hidden bg-background dark:bg-background-dark-default">
      <div className="flex flex-1 flex-col overflow-hidden">
        {/* Top bar */}
        <header className="flex shrink-0 items-center gap-3 border-b border-gray-200 bg-white px-6 py-3 dark:border-gray-700 dark:bg-background-dark-paper">
          <button
            onClick={() => navigate(`/catalog`)}
            className="flex items-center gap-1.5 rounded-md px-2 py-1.5 text-sm text-foreground-secondary hover:bg-gray-100 hover:text-foreground dark:text-foreground-dark-secondary dark:hover:bg-white/10 dark:hover:text-foreground-dark-default transition-colors"
          >
            <ArrowLeft size={16} />
            Back to catalog
          </button>

          <div className="h-5 w-px bg-gray-300 dark:bg-gray-600" />

          {node && (
            <>
              <span className="rounded-md bg-gray-100 px-2 py-0.5 font-mono text-[0.65rem] font-medium text-foreground-secondary dark:bg-white/10 dark:text-foreground-dark-secondary">
                {node.kind}
              </span>
              <h1 className="truncate text-sm font-semibold text-foreground dark:text-foreground-dark-default" title={node.name}>
                {node.name}
              </h1>
            </>
          )}
        </header>

        <div className="flex-1 overflow-y-auto px-6 py-4">
          {loading && (
            <p className="text-sm text-foreground-secondary dark:text-foreground-dark-secondary">
              Loading…
            </p>
          )}

          {error && (
            <p className="text-sm text-red-500">{error}</p>
          )}

          {!loading && !error && node && (
            <div className="flex flex-col gap-6">
              {/* Tabs */}
              <div className="flex gap-1 border-b border-gray-200 dark:border-gray-700">
                {tabs.map(({ value, label }) => (
                  <button
                    key={value}
                    onClick={() => setActiveTab(value)}
                    className={cn(
                      'px-4 py-2 text-sm transition-colors',
                      activeTab === value
                        ? 'border-b-2 border-primary font-semibold text-foreground dark:text-foreground-dark-default'
                        : 'text-foreground-secondary hover:text-foreground dark:text-foreground-dark-secondary dark:hover:text-foreground-dark-default'
                    )}
                  >
                    {label}
                  </button>
                ))}
              </div>

              {/* Tab content */}
              <div>
                {activeTab === 'Graph' && (
                  <div className="h-[500px]">
                    <CatalogGraph rootNode={{ name: node.name, label: node.name }} />
                  </div>
                )}

                {activeTab === 'Properties' && (
                  <PropertiesPanel props={nodeProps(node)} title={`${node.kind} Properties`} />
                )}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
