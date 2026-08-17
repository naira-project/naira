import {
  RawServiceProvider,
  ServiceProviderService,
  ServiceProviderResponse,
} from '@openmfp/portal-server-lib';

const UI_BASE_URL = process.env.NAIRA_UI_BASE_URL;

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

export const SERVICE_PROVIDERS: RawServiceProvider[] = [
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
                label: 'Overview',
                order: 0,
                entityType: 'naira',
                hideFromNav: false,
                url: UI_BASE_URL,
                loadingIndicator: { enabled: false },
              },
              {
                pathSegment: 'dataset',
                label: 'Dataset Catalog',
                order: 1,
                category: { label: 'Catalogs', order: 1 },
                entityType: 'naira',
                hideFromNav: false,
                url: `${UI_BASE_URL}/catalog/dataset`,
                loadingIndicator: { enabled: false },
              },
              {
                pathSegment: 'model',
                label: 'Model Catalog',
                order: 1,
                category: { label: 'Catalogs', order: 1 },
                entityType: 'naira',
                hideFromNav: false,
                url: `${UI_BASE_URL}/catalog/model`,
                loadingIndicator: { enabled: false },
              },
              {
                pathSegment: 'software_catalog',
                label: 'Software Catalog',
                order: 2,
                category: { label: 'Catalogs', order: 1 },
                entityType: 'naira',
                hideFromNav: false,
                url: `${UI_BASE_URL}/catalog/software_catalog`,
                loadingIndicator: { enabled: false },
              }
            ],
          },
        },
      },
    ],
  },
];
