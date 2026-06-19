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
    name: 'getting-started',
    displayName: 'Getting Started',
    creationTimestamp: '',
    contentConfiguration: [
      {
        name: 'getting-started',
        creationTimestamp: '',
        luigiConfigFragment: {
          data: {
            nodes: [
              {
                entityType: 'global',
                pathSegment: 'home',
                label: 'Home',
                icon: 'home',
                hideFromNav: true,
                defineEntity: {
                  id: 'example',
                },
                viewUrl: '/home',
              },
            ],
          },
        },
      },
    ],
  },
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
                pathSegment: 'dashboard',
                label: 'Dashboard',
                entityType: 'example',
                hideFromNav: false,
                url: `${UI_BASE_URL}/index.html/#/`,
                loadingIndicator: { enabled: false },
              },
              {
                pathSegment: 'model-registry',
                label: 'Model Registry',
                entityType: 'example',
                hideFromNav: false,
                keepSelectedForChildren: true,
                url: `${UI_BASE_URL}/index.html/#/model-registry`,
                loadingIndicator: { enabled: false },
                children: [
                  {
                    pathSegment: ':id',
                    label: 'Model Details',
                    entityType: 'example',
                    hideFromNav: true,
                    url: `${UI_BASE_URL}/index.html/#/model-registry/:id`,
                    loadingIndicator: { enabled: false },
                  },
                ],
              },
            ],
          },
        },
      },
    ],
  },
];
