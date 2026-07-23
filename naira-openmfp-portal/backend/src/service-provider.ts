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
                pathSegment: 'dashboard',
                label: 'Dashboard',
                entityType: 'naira',
                hideFromNav: false,
                url: UI_BASE_URL,
                loadingIndicator: { enabled: false },
              },
              {
                pathSegment: 'deployment',
                label: 'Deployment',
                entityType: 'naira',
                hideFromNav: false,
                url: `${UI_BASE_URL}/catalog/kinds/deployment`,
                loadingIndicator: { enabled: false },
              },
              {
                pathSegment: 'hugging_face_repository',
                label: 'Hugging Face Repository',
                entityType: 'naira',
                hideFromNav: false,
                url: `${UI_BASE_URL}/catalog/kinds/hugging_face_repository`,
                loadingIndicator: { enabled: false },
              },
              {
                pathSegment: 'model',
                label: 'Model',
                entityType: 'naira',
                hideFromNav: false,
                url: `${UI_BASE_URL}/catalog/kinds/model`,
                loadingIndicator: { enabled: false },
              },
              {
                pathSegment: 'service',
                label: 'Service',
                entityType: 'naira',
                hideFromNav: false,
                url: `${UI_BASE_URL}/catalog/kinds/service`,
                loadingIndicator: { enabled: false },
              },
            ],
          },
        },
      },
    ],
  },
];
