import { useState, useEffect, useMemo } from 'react';
import { useNavigate } from 'react-router';
import { Search, Layers, Plug } from 'lucide-react';
import { Input } from '../components/ui/input';
import { useKinds } from '../hooks/useKinds';
import { useCatalogNodes } from '../hooks/useCatalogNodes';
import { useRelationSummaries } from '../hooks/useRelationSummaries';
import { usePluginsStatus } from '../hooks/usePluginOperations';
import { NodeResource } from '../lib/catalogApi';
import { derivePlugins } from '../lib/kindUtils';
import KindSelector from '../components/KindSelector';
import PluginTabs from '../components/PluginTabs';
import GenericTable from '../components/GenericTable';
import EmptyState from '../components/EmptyState';
import { PluginsManagerDialog } from '../components/PluginsManagerDialog';
import { formatRelativeTime, latestOperation } from '../lib/utils';


interface CatalogViewProps {
  allowedKinds?: string[];
  heading?: string;
  subheading?: string;
  /** Restrict the Plugins & Ingestion dialog to this subset of plugins. Omit to show every plugin. */
  allowedPlugins?: string[];
}
/**
 * Unified catalog view.
 * Focuses on browsing the catalog: kind selector + resource table.
 * Plugin management (run, history, status) lives in a dedicated dialog.
 */
export default function CatalogView({ allowedKinds, heading, subheading, allowedPlugins }: CatalogViewProps) {
  const navigate = useNavigate();
  const [search, setSearch] = useState('');
  const [pluginsOpen, setPluginsOpen] = useState(false);

  // Kind discovery & selection
  const { kinds, kindsLoading, kindsError, activeKind, setActiveKind, refreshKinds } = useKinds(allowedKinds);

  // Fetch nodes for the active kind
  const { nodes, loading: nodesLoading, error: nodesError } = useCatalogNodes(activeKind ?? '');

  // Per-plugin tab, shown below the kind selector. Resets whenever the kind changes.
  const [activePlugin, setActivePlugin] = useState<string | null>(null);
  useEffect(() => {
    setActivePlugin(null);
  }, [activeKind]);

  const plugins = useMemo(() => derivePlugins(nodes), [nodes]);
  const filteredNodes = useMemo(
    () =>
      activePlugin
        ? nodes.filter((n) => n.pluginClaims?.some((c) => c.plugin === activePlugin))
        : nodes,
    [nodes, activePlugin]
  );

  // Relation summaries — computed whenever the filtered node set changes
  const { relationSummaries } = useRelationSummaries(filteredNodes);

  // Plugin run operations (used only for the compact "last sync" indicator)
  const { operations, refresh: refreshOps } = usePluginsStatus();

  // Keep kinds fresh whenever the plugin dialog reports completed runs.
  const handleRunsCompleted = () => {
    refreshKinds();
    refreshOps();
  };

  // Refresh kinds on mount so data appears without manual interaction.
  useEffect(() => {
    refreshKinds();
  }, [refreshKinds]);

  // Filter kinds by search
  const filteredKinds = kinds.filter((k) =>
    k.toLowerCase().includes(search.toLowerCase())
  );

  const lastSync = useMemo(() => latestOperation(operations), [operations]);

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

          <div className="flex-1" />

          {/* Compact last-sync indicator */}
          {lastSync && (
            <div className="flex items-center gap-1.5 text-xs text-gray-500 dark:text-gray-400">
              <span>
                Last sync: {formatRelativeTime(lastSync.createdAt)}
              </span>
            </div>
          )}

          {/* Plugins & Ingestion button */}
          <button
            onClick={() => setPluginsOpen(true)}
            className="inline-flex items-center gap-2 rounded-md bg-primary px-3 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90"
          >
            <Plug size={16} />
            Plugins & Ingestion
          </button>
        </header>

        <div className="flex flex-1 flex-col overflow-y-auto px-6 py-4">
          {!kindsError && !kindsLoading && kinds.length === 0 ? (
            <EmptyState />
          ) : (
            <div className="mb-6">
              <h1 className="text-xl font-semibold text-foreground dark:text-foreground-dark-default">
                {heading ?? 'Catalog Explorer'}
              </h1>
              <p className="mt-1 mb-4 text-sm text-foreground-secondary dark:text-foreground-dark-secondary">
                {subheading ?? 'Select a resource kind to browse its entries.'}
              </p>

              {kindsError && (
                <div className="mb-4 flex items-center gap-2 text-sm text-red-500">
                  <span>{kindsError}</span>
                  <button onClick={refreshKinds} className="underline hover:no-underline">
                    Retry
                  </button>
                </div>
              )}

              {!kindsError && filteredKinds.length === 0 && !kindsLoading && (
                <div className="mb-4 flex flex-col items-center gap-2 py-6 text-foreground-secondary dark:text-foreground-dark-secondary">
                  <Layers size={32} className="opacity-40" />
                  <p className="text-sm">No kinds match your search.</p>
                </div>
              )}
              {(!allowedKinds || allowedKinds.length > 1) && (
              <KindSelector
                kinds={filteredKinds}
                activeKind={activeKind}
                onSelect={setActiveKind}
                loading={kindsLoading}
              />
              )}
            </div>
          )}

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
                    ({filteredNodes.length} node{filteredNodes.length !== 1 ? 's' : ''})
                  </span>
                )}
              </div>

              {!nodesLoading && !nodesError && plugins.length > 0 && (
                <div className="mb-4">
                  <PluginTabs plugins={plugins} activePlugin={activePlugin} onSelect={setActivePlugin} />
                </div>
              )}

              {nodesError && (
                <p className="text-sm text-red-500">{nodesError}</p>
              )}

              {!nodesLoading && !nodesError && (
                <GenericTable
                  nodes={filteredNodes}
                  kind={activeKind}
                  onSelect={handleSelect}
                  relationSummaries={relationSummaries}
                />
              )}
            </div>
          )}
        </div>
      </div>

      {/* Plugin management dialog */}
      <PluginsManagerDialog
        open={pluginsOpen}
        onClose={() => setPluginsOpen(false)}
        onRunsCompleted={handleRunsCompleted}
        allowedPlugins={allowedPlugins}
      />
    </div>
  );
}
