/**
 * Catalog viewpoints — each maps a URL path segment (matching the portal's
 * nav config in naira-openmfp-portal/backend/src/service-provider.ts) to the
 * kinds and plugins that viewpoint should be scoped to.
 *
 * Add a new viewpoint here and it picks up its own route, kind selector,
 * and Plugins & Ingestion dialog without touching App.tsx.
 */
export interface CatalogViewpoint {
  path: string;
  heading: string;
  subheading: string;
  kinds: string[];
  relatedKinds?: string[];
  columns?: string[];
  plugins: string[];
}

export const CATALOG_VIEWPOINTS: CatalogViewpoint[] = [
  {
    path: 'dataset',
    heading: 'Dataset Catalog',
    subheading: 'Datasets registered in the catalog.',
    kinds: ['dataset'],
    plugins: ['openmetadata'],
  },
  {
    path: 'software_catalog',
    heading: 'Software Catalog',
    subheading: 'Deployments and services running in the cluster.',
    kinds: ['deployment', 'service'],
    plugins: ['depl-calls-svc', 'depl-uses-litellm', 'fluxcd'],
  },
  {
    path: 'model',
    heading: 'Model',
    subheading: 'Models registered in the catalog.',
    kinds: ['model'],
    plugins: ['litellm', 'mlflow'],
  },
  {
    path: 'mcp',
    heading: 'MCP Catalog',
    subheading: 'MCP servers and the tools they expose.',
    kinds: ['mcp_server'],
    relatedKinds: ['mcp_tool'],
    columns: ['endpoint', 'description', 'instructions'],
    plugins: ['mcp-servers', 'litellm'],
  },
];

/**
 * Finds the viewpoint that owns a given node kind, e.g. "dataset" -> the
 * Dataset Catalog viewpoint. Used to route back to a node's parent catalog
 * page without relying on browser history.
 */
export function findViewpointForKind(kind: string): CatalogViewpoint | undefined {
  return CATALOG_VIEWPOINTS.find(
    (viewpoint) =>
      viewpoint.kinds.includes(kind) || (viewpoint.relatedKinds?.includes(kind) ?? false),
  );
}
