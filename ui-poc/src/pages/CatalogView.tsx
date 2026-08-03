import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router';
import { Search, RefreshCw, Layers, Play } from 'lucide-react';
import { Input } from '../components/ui/input';
import { useKinds } from '../hooks/useKinds';
import { useCatalogNodes } from '../hooks/useCatalogNodes';
import { useRelationSummaries } from '../hooks/useRelationSummaries';
import { usePluginOperations, useAllPluginsRun } from '../hooks/usePluginOperations';
import { NodeResource, fetchPlugins } from '../lib/catalogApi';
import KindSelector from '../components/KindSelector';
import GenericTable from '../components/GenericTable';
import { PluginRunButton } from '../components/PluginRunButton';
import { PluginRunHistory } from '../components/PluginRunHistory';

/**
 * Unified catalog view.
 * Shows a kind selector at the top and, when a kind is selected,
 * displays all nodes of that kind in a dynamic table below.
 * Includes plugin run controls and execution history.
 */
export default function CatalogView() {
  const navigate = useNavigate();
  const [search, setSearch] = useState('');
  const [plugins, setPlugins] = useState<string[]>([]);

  // Kind discovery & selection
  const { kinds, kindsLoading, kindsError, activeKind, setActiveKind, refreshKinds } = useKinds();

  // Fetch nodes for the active kind
  const { nodes, loading: nodesLoading, error: nodesError } = useCatalogNodes(activeKind ?? '');

  // Relation summaries — computed whenever nodes change
  const { relationSummaries } = useRelationSummaries(nodes);

  // Plugin run operations
  const { operations: pluginOps, loading: opsLoading, refresh: refreshOps } = usePluginOperations();
  const { running: allRunning, trigger: triggerAll } = useAllPluginsRun();

  // Fetch available plugins on mount
  useEffect(() => {
    fetchPlugins().then(setPlugins).catch(() => {});
  }, []);

  // Refresh kinds after plugin runs complete
  const handleRunAll = async () => {
    await triggerAll();
    refreshKinds();
    refreshOps();
  };

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

          {/* Plugin toolbar */}
          <div className="flex items-center gap-2">
            {/* Run All button */}
            <button
              onClick={handleRunAll}
              disabled={allRunning}
              className="inline-flex items-center gap-2 rounded-md bg-primary px-3 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
            >
              <RefreshCw size={16} className={allRunning ? 'animate-spin' : ''} />
              {allRunning ? 'Running All…' : 'Run All Plugins'}
            </button>

            {/* Per-plugin run buttons */}
            {plugins.map((plugin) => (
              <PluginRunButton key={plugin} pluginName={plugin} running={false} onRun={() => {}} />
            ))}
          </div>
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
                <button onClick={refreshKinds} className="underline hover:no-underline">
                  Retry
                </button>
              </div>
            )}

            {!kindsError && filteredKinds.length === 0 && !kindsLoading && (
              <div className="mb-4 flex flex-col items-center gap-2 py-6 text-foreground-secondary dark:text-foreground-dark-secondary">
                <Layers size={32} className="opacity-40" />
                <p className="text-sm">
                  {kinds.length === 0
                    ? 'No kinds found. Run plugins to populate the catalog.'
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

          {/* Plugin run history */}
          <div className="mb-6">
            <PluginRunHistory
              operations={pluginOps}
              loading={opsLoading}
              onRefresh={refreshOps}
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
                  relationSummaries={relationSummaries}
                />
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
