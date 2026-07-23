import { useParams } from 'react-router';
import { RefreshCw } from 'lucide-react';
import { Badge } from '../components/ui/badge';
import { useCatalogSync } from '../hooks/useCatalogSync';
import KindTable from '../components/KindTable';

/**
 * Focused, single-kind view reached directly from its own sidebar entry
 * (e.g. Deployment, Model). Unlike CatalogView, there is no kind selector —
 * the kind comes from the route and the page shows only that kind's table.
 */
export default function CatalogKindView() {
  const { kind } = useParams<{ kind: string }>();
  const { syncing, syncMessage, syncError, handleSync } = useCatalogSync();

  if (!kind) {
    return null;
  }

  return (
    <div className="flex h-screen overflow-hidden bg-background dark:bg-background-dark-default">
      <div className="flex flex-1 flex-col overflow-hidden">
        <header className="flex shrink-0 items-center gap-3 border-b border-gray-200 bg-white px-6 py-3 dark:border-gray-700 dark:bg-background-dark-paper">
          <h1 className="text-lg font-semibold capitalize text-foreground dark:text-foreground-dark-default">
            {kind}
          </h1>
          {/*<button
            onClick={handleSync}
            disabled={syncing}
            className="ml-auto inline-flex items-center gap-2 rounded-md bg-primary px-3 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
          >
            <RefreshCw size={16} className={syncing ? 'animate-spin' : ''} />
            {syncing ? 'Synchronizing…' : 'Synchronize Data'}
          </button>*/}
          {syncMessage && <Badge variant="success">{syncMessage}</Badge>}
          {syncError && <Badge variant="error">{syncError}</Badge>}
        </header>

        <div className="flex-1 overflow-y-auto px-6 py-4">
          <KindTable kind={kind} />
        </div>
      </div>
    </div>
  );
}
