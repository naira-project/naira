import {
  ChartsProvider,
  generateChartsTheme,
  getTheme,
  SnackbarProvider,
} from '@perses-dev/components';
import { DatasourceStoreProvider, VariableProvider } from '@perses-dev/dashboards';
import { PluginRegistry, TimeRangeProvider } from '@perses-dev/plugin-system';
import type { DurationString, TimeRangeValue } from '@perses-dev/spec';
import React from 'react';
import { RELATED_NODES_CONFIG_BY_KIND } from '../../config/detailTabs';
import { type NodeResource, nodeProps } from '../../lib/catalogApi';
import { useRelatedNodes } from '../RelatedNodes';
import { bedrockPanels } from './catalogPlugins/bedrock';
import { litellmPanels } from './catalogPlugins/litellm';
import { MetricPanel } from './MetricPanel';
import { datasourceApi, pluginLoader } from './pluginLoader';
import type { PanelConfig } from './types';

const chartsTheme = generateChartsTheme(getTheme('light'), {});

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

// A model that no inference endpoint serves (endpoints only exist once a model has traffic) has nothing to plot.
function ModelDashboard({ node }: { node: NodeResource }) {
  const { related: endpoints, loading } = useRelatedNodes(node, RELATED_NODES_CONFIG_BY_KIND.model);
  if (loading || endpoints.length === 0) {
    return null;
  }
  return <DashboardPanels panels={panelsForNode(node)} />;
}

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
        </PluginRegistry>
      </SnackbarProvider>
    </ChartsProvider>
  );
}
