import type {
  DatasourceApi,
  DatasourceResource,
  GlobalDatasourceResource,
} from '@perses-dev/client';

import {
  ChartsProvider,
  generateChartsTheme,
  getTheme,
  SnackbarProvider,
} from '@perses-dev/components';
import { DatasourceStoreProvider, VariableProvider } from '@perses-dev/dashboards';
import {
  dynamicImportPluginLoader,
  getPluginModuleCompoundKey,
  type PluginModuleResource,
  PluginRegistry,
  TimeRangeProvider,
} from '@perses-dev/plugin-system';
import * as prometheusPlugin from '@perses-dev/prometheus-plugin';
import type { DurationString, TimeRangeValue } from '@perses-dev/spec';
import * as timeseriesChartPlugin from '@perses-dev/timeseries-chart-plugin';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import React from 'react';
import { RELATED_CARDS_BY_KIND } from '../config/detailTabs';
import { type NodeResource, nodeProps } from '../lib/catalogApi';
import { MetricPanel } from './MetricPanel';
import { useRelatedNodes } from './RelatedNodes';

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
const datasourceApi = new StaticDatasourceApi();

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

interface PanelConfig {
  title: string;
  query: string;
}

interface LitellmScope {
  // A single deployment. model_id disambiguates deployments that share the same
  // requested_model (e.g. the two idp-claude-sonnet regional entries in litellm.yaml).
  deploymentId?: string;
  // All deployments behind one model alias (the `requested_model` label).
  requestedModel?: string;
}

function litellmPanels({ deploymentId, requestedModel }: LitellmScope): PanelConfig[] {
  const filter = [
    deploymentId ? `, model_id="${deploymentId}"` : '',
    requestedModel ? `, requested_model=${JSON.stringify(requestedModel)}` : '',
  ].join('');
  return [
    {
      title: 'Input Token Rate',
      query: `sum by (requested_model, model_id) (rate(litellm_input_tokens_metric_total{job="litellm", model!~"MCP:.*"${filter}}[5m]))`,
    },
    {
      title: 'Failure Rate per Deployment',
      query: `sum by (requested_model, model_id) (rate(litellm_deployment_failure_responses_total{job="litellm"${filter}}[5m]))`,
    },
    {
      title: 'P95 Latency',
      query: `histogram_quantile(0.95, sum by (le, requested_model, model_id) (rate(litellm_request_total_latency_metric_bucket{job="litellm"${filter}}[5m])))`,
    },
    {
      title: 'Request rate per Deployment',
      query: `sum by (requested_model, model_id) (rate(litellm_deployment_total_requests_total{job="litellm"${filter}}[5m]))`,
    },
  ];
}

// YACE exports each CloudWatch datapoint as a gauge holding the statistic over
// its 5m period (see deploy/dev/stacks/llm-inference/infra/helm/yace-values.yaml),
// so these are plotted as-is rather than rate()d.
// A model node has no region, so without one every region is drawn as its own line.
function bedrockPanels(modelId: string, region?: string): PanelConfig[] {
  const regionFilter = region ? `, region="${region}"` : '';
  const selector = `{dimension_ModelId="${modelId}"${regionFilter}}`;
  return [
    {
      title: 'Invocations (per 5m)',
      query: `sum by (dimension_ModelId, region) (aws_bedrock_invocations_sum${selector})`,
    },
    {
      title: 'Input Tokens (per 5m)',
      query: `sum by (dimension_ModelId, region) (aws_bedrock_input_token_count_sum${selector})`,
    },
    {
      title: 'Output Tokens (per 5m)',
      query: `sum by (dimension_ModelId, region) (aws_bedrock_output_token_count_sum${selector})`,
    },
    {
      title: 'Average Invocation Latency (ms)',
      query: `avg by (dimension_ModelId, region) (aws_bedrock_invocation_latency_average${selector})`,
    },
  ];
}

function panelsForNode(node: NodeResource): PanelConfig[] {
  const props = nodeProps(node);
  const fromBedrock = (node.pluginClaims ?? []).some((claim) => claim.plugin === 'bedrock');
  if (fromBedrock && props.model_id) {
    return bedrockPanels(props.model_id, props.region_name);
  }
  if (node.kind === 'model') {
    // A LiteLLM model page covers every deployment behind the alias. The alias
    // is the path minus its plugin prefix, e.g. "litellm/idp-llama-qwen25-05b".
    return litellmPanels({ requestedModel: node.path.split('/').slice(1).join('/') });
  }
  // The LiteLLM plugin stores the deployment's model_info.id as `id`, which is
  // the `model_id` label on LiteLLM's Prometheus metrics.
  return litellmPanels({ deploymentId: props.id });
}

export function PersesDashboard({ node }: { node: NodeResource }) {
  if (node.kind === 'model') {
    return <ModelDashboard node={node} />;
  }
  return <DashboardPanels panels={panelsForNode(node)} />;
}

// Metrics are recorded per deployment, so a model that no inference endpoint
// serves (endpoints only exist once a model has traffic) has nothing to plot.
// The "Served By" card on the same page already explains that.
function ModelDashboard({ node }: { node: NodeResource }) {
  const { related: endpoints, loading } = useRelatedNodes(node, RELATED_CARDS_BY_KIND.model);
  if (loading || endpoints.length === 0) {
    return null;
  }
  return <DashboardPanels panels={panelsForNode(node)} />;
}

const chartsTheme = generateChartsTheme(getTheme('light'), {});

const pluginLoader = dynamicImportPluginLoader([
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

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: 0,
    },
  },
});

function DashboardPanels({ panels }: { panels: PanelConfig[] }) {
  const [timeRange, setTimeRange] = React.useState<TimeRangeValue>({ pastDuration: '6h' });
  const [refreshInterval, setRefreshInterval] = React.useState<DurationString>('0s');

  return (
    <ChartsProvider chartsTheme={chartsTheme}>
      <SnackbarProvider
        anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
        variant="default"
      >
        <PluginRegistry
          pluginLoader={pluginLoader}
          defaultPluginKinds={{
            Panel: 'TimeSeriesChart',
            TimeSeriesQuery: 'PrometheusTimeSeriesQuery',
          }}
        >
          <QueryClientProvider client={queryClient}>
            <TimeRangeProvider
              timeRange={timeRange}
              refreshInterval={refreshInterval}
              setTimeRange={setTimeRange}
              setRefreshInterval={setRefreshInterval}
            >
              <VariableProvider>
                <DatasourceStoreProvider datasourceApi={datasourceApi}>
                  {panels.map(({ title, query }) => (
                    <MetricPanel key={title} title={title} query={query} />
                  ))}
                </DatasourceStoreProvider>
              </VariableProvider>
            </TimeRangeProvider>
          </QueryClientProvider>
        </PluginRegistry>
      </SnackbarProvider>
    </ChartsProvider>
  );
}
