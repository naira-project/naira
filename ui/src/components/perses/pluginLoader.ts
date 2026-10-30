import {
  dynamicImportPluginLoader,
  getPluginModuleCompoundKey,
  type PluginModuleResource,
} from '@perses-dev/plugin-system';
import * as prometheusPlugin from '@perses-dev/prometheus-plugin';
import * as timeseriesChartPlugin from '@perses-dev/timeseries-chart-plugin';

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
