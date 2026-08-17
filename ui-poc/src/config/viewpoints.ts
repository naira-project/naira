/**
 * Catalog viewpoints — each maps a URL path segment (matching the portal's
 * nav config in naira-openmfp-portal/backend/src/service-provider.ts) to the
 * kinds and plugins that viewpoint should be scoped to.
 *
 * Add a new viewpoint here and it picks up its own route, kind selector,
 * and Plugins & Ingestion dialog without touching App.tsx.
 */
export interface CatalogViewpoint {
  /** URL segment under /catalog, e.g. "model" -> /catalog/model */
  path: string;
  heading: string;
  subheading: string;
  allowedKinds: string[];
  allowedPlugins: string[];
}

export const CATALOG_VIEWPOINTS: CatalogViewpoint[] = [
  {
    path: 'dataset',
    heading: 'Dataset Catalog',
    subheading: 'Datasets registered in the catalog.',
    allowedKinds: ['dataset'],
    allowedPlugins: ['openmetadata'],
  },
  {
    path: 'software_catalog',
    heading: 'Software Catalog',
    subheading: 'Deployments and services running in the cluster.',
    allowedKinds: ['deployment', 'service'],
    allowedPlugins: ['depl-calls-svc', 'depl-uses-litellm', 'fluxcd'],
  },
  {
    path: 'model',
    heading: 'Model',
    subheading: 'Models registered in the catalog.',
    allowedKinds: ['model'],
    allowedPlugins: ['litellm', 'mlflow'],
  },
];
