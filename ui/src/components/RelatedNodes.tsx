import { ChevronRight } from 'lucide-react';
import type { ComponentType, ReactNode } from 'react';
import { useMemo } from 'react';
import { useNavigate } from 'react-router';
import { type CatalogGraphResponse, useCatalogGraph } from '../hooks/useCatalogGraph';
import { encodeCatalogPath, type NodeResource } from '../lib/catalogApi';
import { parsePath } from '../lib/kindUtils';
import { Card } from './ui/card';

/**
 * Describes one set of neighbours reached from a node via a single relation
 * kind/direction, e.g. the tools an MCP server exposes, or the model an
 * inference endpoint serves. Drives both the Tools tab (`layout: 'list'`) and
 * the Properties-tab cross-link cards (`layout: 'cards'`, the default).
 */
export interface RelatedNodesConfig {
  /** Relation kind to filter edges by, e.g. "exposes" or "serves_model". */
  relationKind: string;
  /**
   * 'outgoing' – follow edges leaving this node (fromNode === this node).
   * 'incoming' – follow edges arriving at this node (toNode === this node).
   */
  direction: 'outgoing' | 'incoming';
  /** Heading (cards layout) or item-type label (list layout). */
  title: string;
  /** Icon shown next to each item in list layout. Defaults to a chevron. */
  icon?: ComponentType<{ size?: number; className?: string }>;
  /** Word appended after the item count in list layout, e.g. "exposed". */
  countSuffix?: string;
  /** Shown instead of nothing when list layout has no items. Cards layout renders nothing either way. */
  emptyText?: ReactNode;

  description?: string;
}

interface RelatedNode {
  name: string;
  kind: string;
  path: string;
  displayName: string;
  title?: string;
  description?: string;
}

interface RelatedNodesProps {
  node: NodeResource;
  config: RelatedNodesConfig;
}

/**
 * Neighbours of `node` reached via one relation kind, rendered either as a
 * detail list (Tools tab) or as clickable cross-link cards (Properties tab).
 *
 * Reads the depth-1 graph slice, which the Graph tab also uses, so this doesn't
 * cost an extra request beyond what's already cached for the node.
 */
export default function RelatedNodes({ node, config }: RelatedNodesProps) {
  const navigate = useNavigate();
  const { graph, loading, error } = useCatalogGraph({ name: node.name }, 1);
  const related = useMemo(
    () => relatedNodesFromGraph(graph, node.name, config),
    [graph, node.name, config],
  );

  if (loading) {
    return <p className="text-sm text-muted-foreground">Loading {config.title.toLowerCase()}…</p>;
  }

  if (error) {
    return <p className="text-sm text-red-500">{error}</p>;
  }

  if (related.length === 0) {
    return config.emptyText ? (
      <p className="text-sm text-muted-foreground">{config.emptyText}</p>
    ) : null;
  }

  const goTo = (item: RelatedNode) =>
    navigate(`/catalog/${encodeURIComponent(item.kind)}/${encodeCatalogPath(item.path)}`);

 
  const Icon = config.icon ?? ChevronRight;
  const itemLabel = related.length === 1 ? config.title.replace(/s$/, '') : config.title;

  return (
    <div className="flex flex-col gap-2">
      <p className="text-xs text-muted-foreground">
        {related.length} {itemLabel}
        {config.countSuffix ? ` ${config.countSuffix}` : ''}
      </p>

      <ul className="divide-y divide-gray-200 overflow-hidden rounded-md border border-gray-200">
        {related.map((item) => {
          const description = item.description ?? config.description;

          return (
            <li key={item.name}>
              <button
                type="button"
                onClick={() => goTo(item)}
                className="flex w-full items-start gap-3 bg-card px-4 py-3 text-left transition-colors hover:bg-gray-50"
              >
                <Icon size={15} className="mt-0.5 shrink-0 text-muted-foreground" />

                <div className="min-w-0 flex-1">
                  <div className="flex items-baseline gap-2">
                    <span className="truncate font-mono text-sm font-medium text-foreground">
                      {item.displayName}
                    </span>
                    {item.title && item.title !== item.displayName && (
                      <span className="truncate text-xs text-muted-foreground">{item.title}</span>
                    )}
                  </div>

                  {description && (
                    <p className="mt-0.5 line-clamp-2 text-xs text-muted-foreground">
                      {description}
                    </p>
                  )}
                </div>

                <ChevronRight size={15} className="mt-0.5 shrink-0 text-muted-foreground" />
              </button>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

  /*return (
    <div>
      <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
        {config.title}
      </h3>
      <div className="flex flex-wrap gap-3">
        {related.map((item) => (
          <Card key={item.name} className="w-64 overflow-hidden">
            <button
              type="button"
              onClick={() => goTo(item)}
              className="flex w-full items-center gap-3 px-4 py-3 text-left transition-colors hover:bg-gray-50"
            >
              <div className="min-w-0 flex-1">
                <span className="block w-fit rounded-md bg-gray-100 px-1.5 py-0.5 font-mono text-[0.6rem] font-medium uppercase text-muted-foreground">
                  {item.kind}
                </span>
                <span className="mt-1 block truncate text-sm font-medium text-foreground">
                  {item.displayName}
                </span>
              </div>
              <ChevronRight size={15} className="shrink-0 text-muted-foreground" />
            </button>
          </Card>
        ))}
      </div>
    </div>
  );
}*/

/**
 * Resolves the related nodes out of the depth-1 graph slice: the edges of
 * `config.relationKind` touching `nodeName` on the configured side, resolved
 * against the neighbour nodes the slice already carries.
 */
function relatedNodesFromGraph(
  graph: CatalogGraphResponse,
  nodeName: string,
  config: RelatedNodesConfig,
): RelatedNode[] {
  const neighbourNames = new Set(
    graph.edges
      .filter((edge) => edge.kind === config.relationKind)
      .filter((edge) =>
        config.direction === 'outgoing' ? edge.fromNode === nodeName : edge.toNode === nodeName,
      )
      .map((edge) => (config.direction === 'outgoing' ? edge.toNode : edge.fromNode)),
  );

  return graph.nodes
    .filter((n) => neighbourNames.has(n.name))
    .map((n) => ({
      name: n.name,
      kind: n.kind,
      path: n.path,
      displayName: parsePath(n.path).name,
      title: typeof n.properties?.title === 'string' ? n.properties.title : undefined,
      description:
        typeof n.properties?.description === 'string' ? n.properties.description : undefined,
    }))
    .sort((a, b) => a.displayName.localeCompare(b.displayName));
}
