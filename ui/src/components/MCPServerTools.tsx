import { ChevronRight, Wrench } from 'lucide-react';
import { useMemo } from 'react';
import { useNavigate } from 'react-router';
import { type CatalogGraphResponse, useCatalogGraph } from '../hooks/useCatalogGraph';
import { encodeCatalogPath, type NodeResource } from '../lib/catalogApi';
import { parsePath } from '../lib/kindUtils';

export const MCP_SERVER_KIND = 'mcp_server';
const EXPOSES_RELATION = 'exposes';

interface MCPTool {
  /** Catalog resource name, used to link through to the tool's detail page. */
  name: string;
  kind: string;
  path: string;
  /** Short name, i.e. the last path segment. */
  toolName: string;
  title?: string;
  description?: string;
}

interface MCPServerToolsProps {
  node: NodeResource;
}

/**
 * The tools an MCP server exposes, for the Tools tab of an mcp_server detail
 * page. Owns its own data so the detail page stays kind-agnostic.
 *
 * Reads the depth-1 graph slice, which the Graph tab also uses, so both share
 * one cached request instead of asking for the same relations twice.
 */
export default function MCPServerTools({ node }: MCPServerToolsProps) {
  const navigate = useNavigate();
  const { graph, loading, error } = useCatalogGraph({ name: node.name }, 1);
  const tools = useMemo(() => mcpToolsFromGraph(graph, node.name), [graph, node.name]);

  if (loading) {
    return (
      <p className="text-sm text-foreground-secondary dark:text-foreground-dark-secondary">
        Loading tools…
      </p>
    );
  }

  if (error) {
    return <p className="text-sm text-red-500">{error}</p>;
  }

  if (tools.length === 0) {
    return (
      <p className="text-sm text-foreground-secondary dark:text-foreground-dark-secondary">
        This server exposes no tools. If it is unreachable its tools cannot be read — check the{' '}
        <span className="font-medium">reachable</span> property.
      </p>
    );
  }

  return (
    <div className="flex flex-col gap-2">
      <p className="text-xs text-foreground-secondary dark:text-foreground-dark-secondary">
        {tools.length} {tools.length === 1 ? 'tool' : 'tools'} exposed
      </p>

      <ul className="divide-y divide-gray-200 overflow-hidden rounded-md border border-gray-200 dark:divide-gray-700 dark:border-gray-700">
        {tools.map((tool) => (
          <li key={tool.name}>
            <button
              type="button"
              onClick={() =>
                navigate(
                  `/catalog/${encodeURIComponent(tool.kind)}/${encodeCatalogPath(tool.path)}`,
                )
              }
              className="flex w-full items-start gap-3 bg-white px-4 py-3 text-left transition-colors hover:bg-gray-50 dark:bg-background-dark-paper dark:hover:bg-white/5"
            >
              <Wrench
                size={15}
                className="mt-0.5 shrink-0 text-foreground-secondary dark:text-foreground-dark-secondary"
              />

              <div className="min-w-0 flex-1">
                <div className="flex items-baseline gap-2">
                  <span className="truncate font-mono text-sm font-medium text-foreground dark:text-foreground-dark-default">
                    {tool.toolName}
                  </span>
                  {tool.title && tool.title !== tool.toolName && (
                    <span className="truncate text-xs text-foreground-secondary dark:text-foreground-dark-secondary">
                      {tool.title}
                    </span>
                  )}
                </div>

                {tool.description && (
                  <p className="mt-0.5 line-clamp-2 text-xs text-foreground-secondary dark:text-foreground-dark-secondary">
                    {tool.description}
                  </p>
                )}
              </div>

              <ChevronRight
                size={15}
                className="mt-0.5 shrink-0 text-foreground-secondary dark:text-foreground-dark-secondary"
              />
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
}

/**
 * The tools an MCP server exposes, read out of its depth-1 graph slice: the
 * `exposes` edges leaving the server, resolved against the neighbour nodes the
 * slice already carries.
 */
function mcpToolsFromGraph(graph: CatalogGraphResponse, serverName: string): MCPTool[] {
  const exposed = new Set(
    graph.edges
      .filter((edge) => edge.kind === EXPOSES_RELATION && edge.fromNode === serverName)
      .map((edge) => edge.toNode),
  );

  return graph.nodes
    .filter((node) => exposed.has(node.name))
    .map((node) => ({
      name: node.name,
      kind: node.kind,
      path: node.path,
      toolName: parsePath(node.path).name,
      title: typeof node.properties?.title === 'string' ? node.properties.title : undefined,
      description:
        typeof node.properties?.description === 'string' ? node.properties.description : undefined,
    }))
    .sort((a, b) => a.toolName.localeCompare(b.toolName));
}
