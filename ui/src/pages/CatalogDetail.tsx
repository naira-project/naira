import { ArrowLeft } from 'lucide-react';
import { useState } from 'react';
import { useNavigate, useParams } from 'react-router';
import PropertiesPanel from '../components/PropertiesPanel';
import { detailTabsForKind } from '../config/detailTabs';
import { findViewpointForKind } from '../config/viewpoints';
import { useCatalogDetail } from '../hooks/useCatalogDetail';
import { nodeProps } from '../lib/catalogApi';
import { cn } from '../lib/utils';
import CatalogGraph from './CatalogGraph';

const GRAPH_TAB = 'Graph';
const PROPERTIES_TAB = 'Properties';

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
  const backPath = findViewpointForKind(decodedKind)?.path;

  // Kind-specific tabs come from configuration;
  const kindTabs = detailTabsForKind(decodedKind);
  const landingTab = kindTabs.find((tab) => tab.primary)?.value ?? GRAPH_TAB;
  const [activeTab, setActiveTab] = useState<string>(landingTab);

  const tabs = [
    ...kindTabs.map((tab) => ({ value: tab.value, label: tab.value })),
    { value: GRAPH_TAB, label: GRAPH_TAB },
    { value: PROPERTIES_TAB, label: PROPERTIES_TAB },
  ];

  // One route serves every kind, so the selected tab can outlive the kind that
  // offered it: a tool opened from a server's Tools tab has no Tools tab.
  const currentTab = tabs.some((tab) => tab.value === activeTab) ? activeTab : landingTab;

  return (
    <div className="flex h-screen overflow-hidden bg-background">
      <div className="flex flex-1 flex-col overflow-hidden">
        {/* Top bar */}
        <header className="flex shrink-0 items-center gap-3 border-b border-gray-200 bg-card px-6 py-3 dark:border-gray-700">
          <button
            onClick={() => navigate(backPath ? `/catalog/${backPath}` : '/catalog')}
            className="flex items-center gap-1.5 rounded-md px-2 py-1.5 text-sm text-muted-foreground hover:bg-gray-100 hover:text-foreground dark:hover:bg-white/10 transition-colors"
          >
            <ArrowLeft size={16} />
            Back to catalog
          </button>

          <div className="h-5 w-px bg-gray-300 dark:bg-gray-600" />

          {node && (
            <>
              <span className="rounded-md bg-gray-100 px-2 py-0.5 font-mono text-[0.65rem] font-medium text-muted-foreground dark:bg-white/10">
                {node.kind}
              </span>
              <h1 className="truncate text-sm font-semibold text-foreground" title={node.name}>
                {node.name}
              </h1>
            </>
          )}
        </header>

        <div className="flex-1 overflow-y-auto px-6 py-4">
          {loading && <p className="text-sm text-muted-foreground">Loading…</p>}

          {error && <p className="text-sm text-red-500">{error}</p>}

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
                      currentTab === value
                        ? 'border-b-2 border-primary font-semibold text-foreground'
                        : 'text-muted-foreground hover:text-foreground',
                    )}
                  >
                    {label}
                  </button>
                ))}
              </div>

              {/* Tab content */}
              <div>
                {currentTab === GRAPH_TAB && (
                  <div className="h-[500px]">
                    <CatalogGraph rootNode={{ name: node.name }} />
                  </div>
                )}

                {kindTabs.map(({ value, component: TabComponent }) =>
                  currentTab === value ? <TabComponent key={value} node={node} /> : null,
                )}

                {currentTab === PROPERTIES_TAB && (
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
