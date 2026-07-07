import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router';
import { Search, RefreshCw, Layers } from 'lucide-react';
import { Input } from '../components/ui/input';
import { discoverKinds } from '../lib/kindUtils';
import { useCatalogNodes } from '../hooks/useCatalogNodes';
import { NodeResource } from '../lib/catalogApi';
import KindSelector from '../components/KindSelector';
import GenericTable from '../components/GenericTable';

/**
 * Unified catalog view.
 * Shows a kind selector at the top and, when a kind is selected,
 * displays all nodes of that kind in a dynamic table below.
 */
export default function CatalogView() {
  const navigate = useNavigate();
  const [kinds, setKinds] = useState<string[]>([]);
  const [kindsLoading, setKindsLoading] = useState(true);
  const [kindsError, setKindsError] = useState<string | null>(null);
  const [activeKind, setActiveKind] = useState<string | null>(null);
  const [search, setSearch] = useState('');

  // Fetch nodes for the active kind
  const { nodes, loading: nodesLoading, error: nodesError } = useCatalogNodes(activeKind ?? '');

  // Discover kinds on mount
  const loadKinds = () => {
    setKindsLoading(true);
    setKindsError(null);
    discoverKinds()
      .then((result) => {
        setKinds(result);
        // Auto-select first kind if none selected
        if (!activeKind && result.length > 0) {
          setActiveKind(result[0]);
        }
        setKindsLoading(false);
      })
      .catch(() => {
        setKindsError('Failed to discover kinds');
        setKindsLoading(false);
      });
  };

  useEffect(() => {
    loadKinds();
  }, []);

  // Filter kinds by search
  const filteredKinds = kinds.filter((k) =>
    k.toLowerCase().includes(search.toLowerCase())
  );

  const handleSelect = (node: NodeResource) => {
    navigate(`/catalog/${encodeURIComponent(activeKind!)}/${encodeURIComponent(node.path)}`);
  };

  return (
    <div className="flex h-screen overflow-hidden bg-background dark:bg-background-dark-default">
      <div className="flex flex-1 flex-col overflow-hidden">
        {/* Top bar */}
        <header className="flex shrink-0 items-center gap-3 border-b border-gray-200 bg-white px-6 py-3 dark:border-gray-700 dark:bg-background-dark-paper">
          <Input
            startAdornment={<Search size={16} />}
            placeholder="Search kinds..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="max-w-[320px]"
          />
          <button
            onClick={loadKinds}
            disabled={kindsLoading}
            className="inline-flex items-center gap-2 rounded-md bg-primary px-3 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
          >
            <RefreshCw size={16} className={kindsLoading ? 'animate-spin' : ''} />
            Refresh
          </button>
        </header>

        <div className="flex-1 overflow-y-auto px-6 py-4">
          {/* Kind selector area */}
          <div className="mb-6">
            <h1 className="text-xl font-semibold text-foreground dark:text-foreground-dark-default">
              Catalog Explorer
            </h1>
            <p className="mt-1 mb-4 text-sm text-foreground-secondary dark:text-foreground-dark-secondary">
              Select a resource kind to browse its entries.
            </p>

            {kindsError && (
              <div className="mb-4 flex items-center gap-2 text-sm text-red-500">
                <span>{kindsError}</span>
                <button onClick={loadKinds} className="underline hover:no-underline">
                  Retry
                </button>
              </div>
            )}

            {!kindsError && filteredKinds.length === 0 && !kindsLoading && (
              <div className="mb-4 flex flex-col items-center gap-2 py-6 text-foreground-secondary dark:text-foreground-dark-secondary">
                <Layers size={32} className="opacity-40" />
                <p className="text-sm">
                  {kinds.length === 0
                    ? 'No kinds found. Synchronize data from the Dashboard first.'
                    : 'No kinds match your search.'}
                </p>
              </div>
            )}

            <KindSelector
              kinds={filteredKinds}
              activeKind={activeKind}
              onSelect={setActiveKind}
              loading={kindsLoading}
            />
          </div>

          {/* Node table area */}
          {activeKind && (
            <div>
              <div className="mb-3 flex items-center gap-2">
                <h2 className="text-sm font-semibold text-foreground dark:text-foreground-dark-default">
                  {activeKind}
                </h2>
                {nodesLoading && (
                  <span className="text-xs text-foreground-secondary dark:text-foreground-dark-secondary">
                    Loading…
                  </span>
                )}
                {!nodesLoading && !nodesError && (
                  <span className="text-xs text-foreground-secondary dark:text-foreground-dark-secondary">
                    ({nodes.length} node{nodes.length !== 1 ? 's' : ''})
                  </span>
                )}
              </div>

              {nodesError && (
                <p className="text-sm text-red-500">{nodesError}</p>
              )}

              {!nodesLoading && !nodesError && (
                <GenericTable
                  nodes={nodes}
                  kind={activeKind}
                  onSelect={handleSelect}
                />
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
