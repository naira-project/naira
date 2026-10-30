import type {
  DatasourceApi,
  DatasourceResource,
  GlobalDatasourceResource,
} from '@perses-dev/client';

const PROMETHEUS_URL = '/prometheus';

// Perses resolves a query's datasource through DatasourceStoreProvider. Naira has
// no Perses server, so this serves a single default Prometheus datasource that
// every panel falls back to (the queries don't name one).
// The Prometheus plugin only sends headers set statically on the spec's proxy,
// so the Bearer token is baked into the datasource.
function prometheusDatasource(token: string | null): GlobalDatasourceResource {
  return {
    kind: 'GlobalDatasource',
    metadata: { name: 'prometheus' },
    spec: {
      default: true,
      plugin: {
        kind: 'PrometheusDatasource',
        spec: {
          // Same-origin path proxied to the in-cluster Prometheus by nginx
          // (see nginx.conf.template's /prometheus/ location block).
          directUrl: PROMETHEUS_URL,
          ...(token && {
            proxy: {
              kind: 'HTTPProxy',
              spec: { url: PROMETHEUS_URL, headers: { Authorization: `Bearer ${token}` } },
            },
          }),
        },
      },
    },
  };
}

// In-memory DatasourceApi: exposes only the global Prometheus datasource above and
// no project-scoped ones, so no backend calls are made.
class StaticDatasourceApi implements DatasourceApi {
  private readonly datasource: GlobalDatasourceResource;

  constructor(token: string | null) {
    this.datasource = prometheusDatasource(token);
  }

  getDatasource(): Promise<DatasourceResource | undefined> {
    return Promise.resolve(undefined);
  }

  getGlobalDatasource(): Promise<GlobalDatasourceResource | undefined> {
    return Promise.resolve(this.datasource);
  }

  listDatasources(): Promise<DatasourceResource[]> {
    return Promise.resolve([]);
  }

  listGlobalDatasources(): Promise<GlobalDatasourceResource[]> {
    return Promise.resolve([this.datasource]);
  }
}

export function createDatasourceApi(token: string | null): DatasourceApi {
  return new StaticDatasourceApi(token);
}
