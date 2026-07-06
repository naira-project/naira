import { useParams, useNavigate } from 'react-router';
import { ArrowLeft } from 'lucide-react';
import { useCatalogNodes } from '../hooks/useCatalogNodes';
import GenericTable from '../components/GenericTable';

/**
 * Listing page for all catalog nodes of a given kind.
 * Shows a kind selector at the top and a dynamic table below.
 */
export default function CatalogList() {
  const { kind = '' } = useParams();
  const navigate = useNavigate();
  const decodedKind = decodeURIComponent(kind);
  const { nodes, loading, error } = useCatalogNodes(decodedKind);

  const handleSelect = (node: { name: string; path: string }) => {
    navigate(`/catalog/${encodeURIComponent(decodedKind)}/${encodeURIComponent(node.path)}`);
  };

  return (
    <div className="flex h-screen overflow-hidden bg-background dark:bg-background-dark-default">
      <div className="flex flex-1 flex-col overflow-hidden">
        {/* Top bar */}
        <header className="flex shrink-0 items-center gap-3 border-b border-gray-200 bg-white px-6 py-3 dark:border-gray-700 dark:bg-background-dark-paper">
          <button
            onClick={() => navigate('/catalog')}
            className="flex items-center gap-1.5 rounded-md px-2 py-1.5 text-sm text-foreground-secondary hover:bg-gray-100 hover:text-foreground dark:text-foreground-dark-secondary dark:hover:bg-white/10 dark:hover:text-foreground-dark-default transition-colors"
          >
            <ArrowLeft size={16} />
            All Kinds
          </button>

          <div className="h-5 w-px bg-gray-300 dark:bg-gray-600" />

          <h1 className="text-sm font-semibold text-foreground dark:text-foreground-dark-default">
            {decodedKind}
          </h1>
        </header>

        <div className="flex-1 overflow-y-auto px-6 py-4">
          {loading && (
            <p className="text-sm text-foreground-secondary dark:text-foreground-dark-secondary">
              Loading {decodedKind} nodes…
            </p>
          )}

          {error && (
            <p className="text-sm text-red-500">{error}</p>
          )}

          {!loading && !error && (
            <GenericTable
              nodes={nodes}
              kind={decodedKind}
              onSelect={handleSelect}
            />
          )}
        </div>
      </div>
    </div>
  );
}
