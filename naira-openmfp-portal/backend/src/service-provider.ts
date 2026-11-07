import { Logger } from '@nestjs/common';
import {
  LuigiNode,
  RawServiceProvider,
  ServiceProviderService,
  ServiceProviderResponse,
} from '@openmfp/portal-server-lib';
import { loadWikiNodes } from './wikis.js';

const UI_BASE_URL = process.env.NAIRA_UI_BASE_URL;

// Not a literal at the use site: LuigiNodeCategory does not declare Luigi's isGroup.
const CATALOGS_CATEGORY = { id: 'catalogs', label: 'Catalogs', isGroup: true };
const EXPLORE_CATEGORY = { id: 'explore', label: 'Explore', isGroup: true};
const DOCS_CATEGORY = { id: 'docs', label: 'Docs', isGroup: true};

export class ServiceProviderServiceImpl implements ServiceProviderService {
  async getServiceProviders(
    _token: string,
    _entities: string[],
    _context: Record<string, any>,
  ): Promise<ServiceProviderResponse> {
    return Promise.resolve({
      rawServiceProviders: SERVICE_PROVIDERS,
    });
  }
}

export const serviceProviders = (wikiNodes: LuigiNode[]): RawServiceProvider[] => [
  {
    name: 'naira-catalog',
    displayName: 'Naira Catalog',
    creationTimestamp: new Date().toISOString(),
    contentConfiguration: [
      {
        name: 'catalog',
        creationTimestamp: new Date().toISOString(),
        luigiConfigFragment: {
          data: {
            nodes: [
              {
                entityType: 'global',
                pathSegment: 'naira',
                hideFromNav: true,
                defineEntity: {
                  id: 'naira',
                },
                children: [],
              },
              {
                pathSegment: 'overview',
                icon: 'assets/icons/overview.svg',
                label: 'Overview',
                order: 0,
                entityType: 'naira',
                hideFromNav: false,
                url: UI_BASE_URL,
                loadingIndicator: { enabled: false },
              },
              {
                pathSegment: 'plugins',
                icon: 'assets/icons/plugins.svg',
                label: 'Plugins',
                order: 1,
                entityType: 'naira',
                hideFromNav: false,
                url: `${UI_BASE_URL}/plugins`,
                loadingIndicator: { enabled: false },
              },
              {
                pathSegment: 'model',
                icon: 'assets/icons/model_catalog.svg',
                label: 'Model Catalog',
                order: 2,
                category: CATALOGS_CATEGORY,
                entityType: 'naira',
                hideFromNav: false,
                url: `${UI_BASE_URL}/catalog/model`,
                loadingIndicator: { enabled: false },
              },
              {
                pathSegment: 'mcp',
                icon: 'assets/icons/mcp_catalog.svg',
                label: 'MCP Catalog',
                order: 3,
                category: CATALOGS_CATEGORY,
                entityType: 'naira',
                hideFromNav: false,
                url: `${UI_BASE_URL}/catalog/mcp`,
                loadingIndicator: { enabled: false },
              },
              {
                pathSegment: 'dataset',
                icon: 'assets/icons/dataset_catalog.svg',
                label: 'Dataset Catalog',
                order: 4,
                category: CATALOGS_CATEGORY,
                entityType: 'naira',
                hideFromNav: false,
                url: `${UI_BASE_URL}/catalog/dataset`,
                loadingIndicator: { enabled: false },
              },
              {
                pathSegment: 'inference_endpoints',
                icon: 'assets/icons/inference_endpoints_catalog.svg',
                label: 'Inference Endpoints Catalog',
                order: 5,
                category: CATALOGS_CATEGORY,
                entityType: 'naira',
                hideFromNav: false,
                url: `${UI_BASE_URL}/catalog/inference_endpoints`,
                loadingIndicator: { enabled: false },
              },
              {
                pathSegment: 'software_catalog',
                icon: 'assets/icons/software_catalog.svg',
                label: 'Software Catalog',
                order: 6,
                category: CATALOGS_CATEGORY,
                entityType: 'naira',
                hideFromNav: false,
                url: `${UI_BASE_URL}/catalog/software_catalog`,
                loadingIndicator: { enabled: false },
              },
              {
                pathSegment: 'tech-radar',
                icon: 'assets/icons/tech_radar.svg',
                label: 'Tech Radar',
                order: 7,
                category: EXPLORE_CATEGORY,
                entityType: 'naira',
                hideFromNav: false,
                url: `${UI_BASE_URL}/tech-radar`,
                loadingIndicator: { enabled: false },
              },
              {
                pathSegment: 'coming-soon',
                icon: 'assets/icons/more_coming_soon.svg',
                label: 'More coming soon',
                order: 8,
                category: EXPLORE_CATEGORY,
                entityType: 'naira',
                hideFromNav: false,
                url: `${UI_BASE_URL}/more_coming_soon`,
                loadingIndicator: { enabled: false },
              },
              ...wikiNodes,
            ],
          },
        },
      },
    ],
  },
];

const portalNodes = serviceProviders([]).flatMap((provider) =>
  provider.contentConfiguration.flatMap((content) => content.luigiConfigFragment.data.nodes),
);

export const SERVICE_PROVIDERS = serviceProviders(
  loadWikiNodes(process.env.NAIRA_WIKIS_FILE, {
    category: DOCS_CATEGORY,
    firstOrder: Math.max(...portalNodes.map((node) => node.order ?? 0)) + 1,
    reservedIds: new Set(portalNodes.map((node) => node.pathSegment)),
    log: (message) => new Logger('Wikis').error(message),
  }),
);
