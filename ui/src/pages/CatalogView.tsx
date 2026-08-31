import { Layers, Search } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router';
import GenericTable from '@/components/GenericTable';
import KindSelector from '../components/KindSelector';
import PluginTabs from '../components/PluginTabs';
import EmptyState from '../components/states/EmptyState';
import PluginSyncState from '../components/states/PluginSyncState';
import { Input } from '../components/ui/input';
import { useCatalogNodes } from '../hooks/useCatalogNodes';
import { useKinds } from '../hooks/useKinds';
import { usePluginsStatus } from '../hooks/usePluginOperations';
import { useRelationSummaries } from '../hooks/useRelationSummaries';
import type { NodeResource } from '../lib/catalogApi';
import { derivePlugins } from '../lib/kindUtils';
import { formatRelativeTime, latestOperation } from '../lib/utils';

interface CatalogViewProps {
  viewpointKinds?: string[];
  heading?: string;
  subheading?: string;
  /** Plugins relevant to this viewpoint; used for the empty-state message and its link to /plugins. */
  viewpointPlugins?: string[];
  /** Plugin property columns to show, in order; defaults to all of them. */
  viewpointColumns?: string[];
}
/**
 * Unified catalog view.
 * Focuses on browsing the catalog: kind selector + resource table.
 * Plugin management (run, history, status) lives on its own dedicated page (see PluginsPage).
 */
export default function CatalogView({
  viewpointKinds,
  heading,
  subheading,
  viewpointPlugins,
  viewpointColumns,
}: CatalogViewProps) {
  const navigate = useNavigate();
  const [search, setSearch] = useState('');

  // Kind discovery & selection
  const { kinds, kindsLoading, kindsError, activeKind, setActiveKind, refreshKinds } =
    useKinds(viewpointKinds);

  // Fetch nodes for the active kind
  const { nodes, loading: nodesLoading, error: nodesError } = useCatalogNodes(activeKind ?? '');

  // Per-plugin tab, shown below the kind selector. Resets whenever the kind changes.
  const [activePlugin, setActivePlugin] = useState<string | null>(null);
  useEffect(() => {
    setActivePlugin(null);
  }, []);

  const plugins = useMemo(() => derivePlugins(nodes), [nodes]);
  const filteredNodes = useMemo(
    () =>
      activePlugin
        ? nodes.filter((n) => n.pluginClaims?.some((c) => c.plugin === activePlugin))
        : nodes,
    [nodes, activePlugin],
  );

  // Relation summaries — computed whenever the filtered node set changes
  const { relationSummaries } = useRelationSummaries(filteredNodes);

  // Plugin run operations (used only for the compact "last sync" indicator)
  const { operations } = usePluginsStatus();

  // Filter kinds by search
  const filteredKinds = kinds.filter((k) => k.toLowerCase().includes(search.toLowerCase()));

  const lastSync = useMemo(() => latestOperation(operations), [operations]);

  // Whether a specific viewpoint's plugin(s) have completed at least one successful sync.
  // Distinguishes "never synced" (show PluginSyncState) from "synced, but no data present"
  // (show EmptyState)
  const hasSyncedViewpointPlugin = useMemo(
    () =>
      operations.some(
        (op) =>
          op.state === 'SUCCEEDED' && (!viewpointPlugins || viewpointPlugins.includes(op.plugin)),
      ),
    [operations, viewpointPlugins],
  );

  const handleSelect = (node: NodeResource) => {
    if (!activeKind) {
      return;
    }
    navigate(`/catalog/${encodeURIComponent(activeKind)}/${encodeURIComponent(node.path)}`);
  };

  return (
    <div className="flex h-screen overflow-hidden bg-background">
      <div className="flex flex-1 flex-col overflow-hidden">
        {/* Top bar */}
        <header className="flex shrink-0 items-center gap-3 border-b border-gray-200 bg-card px-6 py-3">
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
            <div className="flex items-center gap-1.5 text-xs text-gray-500">
              <span>Last sync: {formatRelativeTime(lastSync.createdAt)}</span>
            </div>
          )}
        </header>

        <div className="flex flex-1 flex-col overflow-y-auto px-6 py-4">
          {!kindsError && !kindsLoading && kinds.length === 0 ? (
            hasSyncedViewpointPlugin ? (
              <EmptyState />
            ) : (
              <PluginSyncState pluginNames={viewpointPlugins} />
            )
          ) : (
            <div className="mb-6">
              <h1 className="text-xl font-semibold text-foreground">
                {heading ?? 'Catalog Explorer'}
              </h1>
              <p className="mt-1 mb-4 text-sm text-muted-foreground">
                {subheading ?? 'Select a resource kind to browse its entries.'}
              </p>

              {kindsError && (
                <div className="mb-4 flex items-center gap-2 text-sm text-red-500">
                  <span>{kindsError}</span>
                  <button
                    type="button"
                    onClick={refreshKinds}
                    className="underline hover:no-underline"
                  >
                    Retry
                  </button>
                </div>
              )}

              {!kindsError && filteredKinds.length === 0 && !kindsLoading && (
                <div className="mb-4 flex flex-col items-center gap-2 py-6 text-muted-foreground">
                  <Layers size={32} className="opacity-40" />
                  <p className="text-sm">No kinds match your search.</p>
                </div>
              )}
              {(!viewpointKinds || viewpointKinds.length > 1) && (
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
                <h2 className="text-sm font-semibold text-foreground">{activeKind}</h2>
                {nodesLoading && <span className="text-xs text-muted-foreground">Loading…</span>}
                {!nodesLoading && !nodesError && (
                  <span className="text-xs text-muted-foreground">
                    ({filteredNodes.length} node{filteredNodes.length !== 1 ? 's' : ''})
                  </span>
                )}
              </div>

              {!nodesLoading && !nodesError && plugins.length > 0 && (
                <div className="mb-4">
                  <PluginTabs
                    plugins={plugins}
                    activePlugin={activePlugin}
                    onSelect={setActivePlugin}
                  />
                </div>
              )}

              {nodesError && <p className="text-sm text-red-500">{nodesError}</p>}

              {!nodesLoading && !nodesError && (
                <GenericTable
                  nodes={filteredNodes}
                  kind={activeKind}
                  onSelect={handleSelect}
                  relationSummaries={relationSummaries}
                  columns={viewpointColumns}
                />
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
