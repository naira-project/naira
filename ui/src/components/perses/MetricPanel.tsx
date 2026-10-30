import { Panel } from '@perses-dev/dashboards';
import { DataQueriesProvider } from '@perses-dev/plugin-system';

interface MetricPanelProps {
  title: string;
  query: string;
}

export function MetricPanel({ title, query }: MetricPanelProps) {
  return (
    <DataQueriesProvider
      definitions={[
        {
          kind: 'TimeSeriesQuery',
          spec: {
            plugin: {
              kind: 'PrometheusTimeSeriesQuery',
              spec: { query },
            },
          },
        },
      ]}
    >
      <h3 className="mb-2 text-sm font-semibold text-foreground">{title}</h3>
      <div style={{ height: 300, width: '100%', position: 'relative' }}>
        <Panel
          panelOptions={{ hideHeader: true }}
          definition={{
            kind: 'Panel',
            spec: {
              display: { name: title },
              plugin: {
                kind: 'TimeSeriesChart',
                spec: {
                  legend: { position: 'bottom', size: 'medium' },
                },
              },
            },
          }}
        />
      </div>
    </DataQueriesProvider>
  );
}
