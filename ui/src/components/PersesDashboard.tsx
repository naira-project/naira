import React from "react";

import {
  ChartsProvider,
  generateChartsTheme,
  getTheme,
  SnackbarProvider,
} from "@perses-dev/components";
import {
  dynamicImportPluginLoader,
  getPluginModuleCompoundKey,
  PluginModuleResource,
  PluginRegistry,
  TimeRangeProvider,
} from "@perses-dev/plugin-system";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { DatasourceStoreProvider, VariableProvider } from "@perses-dev/dashboards";
import { DurationString, TimeRangeValue } from "@perses-dev/spec";
import {
  DashboardResource,
  GlobalDatasourceResource,
  DatasourceResource,
  DatasourceApi,
} from "@perses-dev/client";
import * as prometheusPlugin from "@perses-dev/prometheus-plugin";
import * as timeseriesChartPlugin from "@perses-dev/timeseries-chart-plugin";
import { type NodeResource, nodeProps } from "../lib/catalogApi";
import { MetricPanel } from "./MetricPanel";

const fakeDatasource: GlobalDatasourceResource = {
  kind: "GlobalDatasource",
  metadata: { name: "hello" },
  spec: {
    default: true,
    plugin: {
      kind: "PrometheusDatasource",
      spec: {
        // Same-origin path proxied to the in-cluster Prometheus by nginx
        // (see ui/nginx.conf.template's /prometheus/ location block).
        directUrl: "/prometheus",
      },
    },
  },
};

class DatasourceApiImpl implements DatasourceApi {
  getDatasource(): Promise<DatasourceResource | undefined> {
    return Promise.resolve(undefined);
  }

  getGlobalDatasource(): Promise<GlobalDatasourceResource | undefined> {
    return Promise.resolve(fakeDatasource);
  }

  listDatasources(): Promise<DatasourceResource[]> {
    return Promise.resolve([]);
  }

  listGlobalDatasources(): Promise<GlobalDatasourceResource[]> {
    return Promise.resolve([fakeDatasource]);
  }

  buildProxyUrl(): string {
    return "/prometheus";
  }
}
export const fakeDatasourceApi = new DatasourceApiImpl();
export const fakeDashboard = {
  kind: "Dashboard",
  metadata: { name: "litellm-endpoint-metrics" },
  spec: {},
} as DashboardResource;

// dynamicImportPluginLoader's registry indexes each loaded module by the exact
// compound key string (kind:name:registry:version), not by the module's plain
// named exports (see @perses-dev/plugin-system's mock-plugin-registry.ts).
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

// model_id disambiguates deployments that share the same requested_model
// (e.g. the two idp-claude-sonnet regional entries in litellm.yaml).
function litellmPanels(modelId?: string): PanelConfig[] {
  const filter = modelId ? `, model_id="${modelId}"` : "";
  return [
    {
      title: "Input Token Rate",
      query: `sum by (requested_model, model_id) (rate(litellm_input_tokens_metric_total{job="litellm", model!~"MCP:.*"${filter}}[5m]))`,
    },
    {
      title: "Failure Rate per Deployment",
      query: `sum by (requested_model, model_id) (rate(litellm_deployment_failure_responses_total{job="litellm"${filter}}[5m]))`,
    },
    {
      title: "P95 Latency",
      query: `histogram_quantile(0.95, sum by (le, requested_model, model_id) (rate(litellm_request_total_latency_metric_bucket{job="litellm"${filter}}[5m])))`,
    },
    {
      title: "Request rate per Deployment",
      query: `sum by (requested_model, model_id) (rate(litellm_deployment_total_requests_total{job="litellm"${filter}}[5m]))`,
    },
  ];
}

// YACE exports each CloudWatch datapoint as a gauge holding the statistic over
// its 5m period (see deploy/dev/stacks/llm-inference/infra/helm/yace-values.yaml),
// so these are plotted as-is rather than rate()d.
function bedrockPanels(modelId: string, region: string): PanelConfig[] {
  const selector = `{dimension_ModelId="${modelId}", region="${region}"}`;
  return [
    {
      title: "Invocations (per 5m)",
      query: `sum by (dimension_ModelId, region) (aws_bedrock_invocations_sum${selector})`,
    },
    {
      title: "Input Tokens (per 5m)",
      query: `sum by (dimension_ModelId, region) (aws_bedrock_inputtokencount_sum${selector})`,
    },
    {
      title: "Output Tokens (per 5m)",
      query: `sum by (dimension_ModelId, region) (aws_bedrock_outputtokencount_sum${selector})`,
    },
    {
      title: "Average Invocation Latency (ms)",
      query: `avg by (dimension_ModelId, region) (aws_bedrock_invocationlatency_average${selector})`,
    },
  ];
}

function panelsForNode(node: NodeResource): PanelConfig[] {
  const props = nodeProps(node);
  const fromBedrock = (node.pluginClaims ?? []).some((claim) => claim.plugin === "bedrock");
  if (fromBedrock && props.model_id && props.region) {
    return bedrockPanels(props.model_id, props.region);
  }
  return litellmPanels(props.model_id);
}

export function PersesDashboard({ node }: { node: NodeResource }) {
  const [timeRange, setTimeRange] = React.useState<TimeRangeValue>({ pastDuration: "30m" });
  const [refreshInterval, setRefreshInterval] = React.useState<DurationString>("0s");

  const panels = panelsForNode(node);
  const muiTheme = getTheme("light");
  const chartsTheme = generateChartsTheme(muiTheme, {});
  const pluginLoader = dynamicImportPluginLoader([
    {
      resource: prometheusPlugin.getPluginModule(),
      importPlugin: () =>
        Promise.resolve(
          toKeyedPluginModule(prometheusPlugin.getPluginModule(), prometheusPlugin),
        ),
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
  return (
      <ChartsProvider chartsTheme={chartsTheme}>
        <SnackbarProvider
          anchorOrigin={{ vertical: "bottom", horizontal: "right" }}
          variant="default"
        >
          <PluginRegistry
            pluginLoader={pluginLoader}
            defaultPluginKinds={{
              Panel: "TimeSeriesChart",
              TimeSeriesQuery: "PrometheusTimeSeriesQuery",
            }}
          >
            <QueryClientProvider client={queryClient}>
              <TimeRangeProvider timeRange={timeRange} refreshInterval={refreshInterval} setTimeRange={setTimeRange} setRefreshInterval={setRefreshInterval}>
                <VariableProvider>
                  <DatasourceStoreProvider
                    datasourceApi={fakeDatasourceApi}
                  >
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