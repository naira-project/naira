import type {
  DatasourceApi,
  DatasourceResource,
  GlobalDatasourceResource,
} from '@perses-dev/client';
import {
  dynamicImportPluginLoader,
  getPluginModuleCompoundKey,
  type PluginModuleResource,
} from '@perses-dev/plugin-system';
import * as prometheusPlugin from '@perses-dev/prometheus-plugin';
import * as timeseriesChartPlugin from '@perses-dev/timeseries-chart-plugin';

// Perses resolves a query's datasource through DatasourceStoreProvider. Naira has
// no Perses server, so this serves a single default Prometheus datasource that
// every panel falls back to (the queries don't name one).
const prometheusDatasource: GlobalDatasourceResource = {
  kind: 'GlobalDatasource',
  metadata: { name: 'prometheus' },
  spec: {
    default: true,
    plugin: {
      kind: 'PrometheusDatasource',
      spec: {
        // Same-origin path proxied to the in-cluster Prometheus by nginx
        // (see nginx.conf.template's /prometheus/ location block).
        directUrl: '/prometheus',
      },
    },
  },
};

// In-memory DatasourceApi: exposes only the global Prometheus datasource above and
// no project-scoped ones, so no backend calls are made.
class StaticDatasourceApi implements DatasourceApi {
  getDatasource(): Promise<DatasourceResource | undefined> {
    return Promise.resolve(undefined);
  }

  getGlobalDatasource(): Promise<GlobalDatasourceResource | undefined> {
    return Promise.resolve(prometheusDatasource);
  }

  listDatasources(): Promise<DatasourceResource[]> {
    return Promise.resolve([]);
  }

  listGlobalDatasources(): Promise<GlobalDatasourceResource[]> {
    return Promise.resolve([prometheusDatasource]);
  }
}
export const datasourceApi = new StaticDatasourceApi();

// dynamicImportPluginLoader's registry indexes each loaded module by the exact
// compound key string (kind:name:registry:version), not by the module's plain
// named exports.
// This rebuilds that keyed shape from a plain `import * as` namespace, relying
// on each plugin's named export matching its declared spec.name (true for all
// @perses-dev/* plugin packages).
function toKeyedPluginModule(
  resource: PluginModuleResource,
  moduleNamespace: Record<string, unknown>,
): Record<string, unknown> {
  const keyed: Record<string, unknown> = {};
  for (const plugin of resource.spec.plugins) {
    const key = getPluginModuleCompoundKey({
      kind: plugin.kind,
      name: plugin.spec.name,
      registry: resource.metadata.registry,
      version: resource.metadata.version,
    });
    keyed[key] = moduleNamespace[plugin.spec.name];
  }
  return keyed;
}

// Tells Perses's PluginRegistry which plugins exist and how to load them.
// The two plugins we need are bundled statically. Each entry pairs a
// plugin's module manifest (`resource`, which declares the plugin kinds/names it
// provides) with an `importPlugin` that resolves to the module itself, rewrapped
// by toKeyedPluginModule into the shape the registry expects.
export const pluginLoader = dynamicImportPluginLoader([
  {
    resource: prometheusPlugin.getPluginModule(),
    importPlugin: () =>
      Promise.resolve(toKeyedPluginModule(prometheusPlugin.getPluginModule(), prometheusPlugin)),
  },
  {
    resource: timeseriesChartPlugin.getPluginModule(),
    importPlugin: () =>
      Promise.resolve(
        toKeyedPluginModule(timeseriesChartPlugin.getPluginModule(), timeseriesChartPlugin),
      ),
  },
]);
