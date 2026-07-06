import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router';
import { Layers, RefreshCw } from 'lucide-react';
import { Search } from 'lucide-react';
import { Input } from '../components/ui/input';
import { Badge } from '../components/ui/badge';
import { discoverKinds } from '../lib/kindUtils';
import KindSelector from '../components/KindSelector';

/**
 * Catalog Overview page.
 * Discovers available kinds from the catalog API and lets the user
 * pick one to browse.
 */
export default function CatalogOverview() {
  const navigate = useNavigate();
  const [kinds, setKinds] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState('');

  const loadKinds = () => {
    setLoading(true);
    setError(null);
    discoverKinds()
      .then((result) => {
        setKinds(result);
        setLoading(false);
      })
      .catch(() => {
        setError('Failed to discover kinds');
        setLoading(false);
      });
  };

  useEffect(() => {
    loadKinds();
  }, []);

  const filteredKinds = kinds.filter((k) =>
    k.toLowerCase().includes(search.toLowerCase())
  );

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
            disabled={loading}
            className="inline-flex items-center gap-2 rounded-md bg-primary px-3 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
          >
            <RefreshCw size={16} className={loading ? 'animate-spin' : ''} />
            Refresh
          </button>
        </header>

        <div className="flex-1 overflow-y-auto px-6 py-6">
          <div className="mb-6">
            <h1 className="text-xl font-semibold text-foreground dark:text-foreground-dark-default">
              Catalog Explorer
            </h1>
            <p className="mt-1 text-sm text-foreground-secondary dark:text-foreground-dark-secondary">
              Select a resource kind to browse its entries.
            </p>
          </div>

          {loading && (
            <div className="flex gap-2">
              <div className="h-8 w-24 animate-pulse rounded-lg bg-gray-200 dark:bg-white/10" />
              <div className="h-8 w-20 animate-pulse rounded-lg bg-gray-200 dark:bg-white/10" />
              <div className="h-8 w-28 animate-pulse rounded-lg bg-gray-200 dark:bg-white/10" />
            </div>
          )}

          {error && (
            <div className="flex items-center gap-2 text-sm text-red-500">
              <span>{error}</span>
              <button onClick={loadKinds} className="underline hover:no-underline">
                Retry
              </button>
            </div>
          )}

          {!loading && !error && filteredKinds.length === 0 && (
            <div className="flex flex-col items-center gap-3 py-12 text-foreground-secondary dark:text-foreground-dark-secondary">
              <Layers size={40} className="opacity-40" />
              <p className="text-sm">
                {kinds.length === 0
                  ? 'No kinds found. Synchronize data from the Dashboard first.'
                  : 'No kinds match your search.'}
              </p>
            </div>
          )}

          {!loading && !error && filteredKinds.length > 0 && (
            <div className="space-y-3">
              {filteredKinds.map((kind) => (
                <button
                  key={kind}
                  onClick={() => navigate(`/catalog/${encodeURIComponent(kind)}`)}
                  className="flex w-full items-center gap-3 rounded-lg border border-gray-200 bg-white px-4 py-3 text-left transition-colors hover:bg-gray-50 dark:border-gray-700 dark:bg-background-dark-paper dark:hover:bg-white/5"
                >
                  <Layers size={20} className="shrink-0 text-primary" />
                  <div className="flex-1">
                    <span className="text-sm font-semibold text-foreground dark:text-foreground-dark-default">
                      {kind}
                    </span>
                  </div>
                  <Badge variant="secondary" className="text-[0.65rem]">
                    Browse
                  </Badge>
                </button>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
