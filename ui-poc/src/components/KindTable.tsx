import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router';
import { Search } from 'lucide-react';
import { Input } from './ui/input';
import { useCatalogNodes } from '../hooks/useCatalogNodes';
import { useRelationSummaries } from '../hooks/useRelationSummaries';
import { NodeResource, nodeProps } from '../lib/catalogApi';
import { derivePlugins, parsePath } from '../lib/kindUtils';
import PluginTabs from './PluginTabs';
import GenericTable from './GenericTable';

interface KindTableProps {
  kind: string;
}

/**
 * Fetches and renders all nodes of a single kind: a plugin tab bar (when more
 * than one plugin claims nodes of this kind), a search box that filters rows
 * by name, namespace, or plugin property values, and the resulting table.
 */
export default function KindTable({ kind }: KindTableProps) {
  const navigate = useNavigate();
  const [search, setSearch] = useState('');

  const { nodes, loading: nodesLoading, error: nodesError } = useCatalogNodes(kind);

  // Per-plugin tab, shown below the search box. Resets whenever the kind changes.
  const [activePlugin, setActivePlugin] = useState<string | null>(null);
  useEffect(() => {
    setActivePlugin(null);
  }, [kind]);

  const plugins = useMemo(() => derivePlugins(nodes), [nodes]);
  const pluginFilteredNodes = useMemo(
    () =>
      activePlugin
        ? nodes.filter((n) => n.pluginClaims?.some((c) => c.plugin === activePlugin))
        : nodes,
    [nodes, activePlugin]
  );

  const filteredNodes = pluginFilteredNodes.filter((node) => {
    if (!search) return true;
    const term = search.toLowerCase();
    const parsed = parsePath(node.path);
    const props = nodeProps(node);
    return (
      (parsed?.name ?? node.name).toLowerCase().includes(term) ||
      (parsed?.namespace ?? '').toLowerCase().includes(term) ||
      Object.values(props).some((v) => v.toLowerCase().includes(term))
    );
  });

  // Relation summaries — computed whenever the filtered node set changes
  const { relationSummaries } = useRelationSummaries(filteredNodes);

  const handleSelect = (node: NodeResource) => {
    navigate(`/catalog/${encodeURIComponent(kind)}/${encodeURIComponent(node.path)}`);
  };

  return (
    <div>
      <div className="mb-4">
        <Input
          startAdornment={<Search size={16} />}
          placeholder={`Search ${kind}...`}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="max-w-[320px]"
        />
      </div>

      <div className="mb-3 flex items-center gap-2">
        {nodesLoading && (
          <span className="text-xs text-foreground-secondary dark:text-foreground-dark-secondary">
            Loading…
          </span>
        )}
        {!nodesLoading && !nodesError && (
          <span className="text-xs text-foreground-secondary dark:text-foreground-dark-secondary">
            ({filteredNodes.length} node{filteredNodes.length !== 1 ? 's' : ''}
            {search ? ` of ${pluginFilteredNodes.length}` : ''})
          </span>
        )}
      </div>

      {!nodesLoading && !nodesError && plugins.length > 0 && (
        <div className="mb-4">
          <PluginTabs plugins={plugins} activePlugin={activePlugin} onSelect={setActivePlugin} />
        </div>
      )}

      {nodesError && <p className="text-sm text-red-500">{nodesError}</p>}

      {!nodesLoading && !nodesError && (
        <GenericTable
          nodes={filteredNodes}
          kind={kind}
          onSelect={handleSelect}
          relationSummaries={relationSummaries}
        />
      )}
    </div>
  );
}
