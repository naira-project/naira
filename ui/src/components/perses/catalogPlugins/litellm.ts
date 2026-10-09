import type { PanelConfig } from '../types';

export interface LitellmScope {
  // A single deployment. model_id disambiguates deployments that share the same
  // requested_model (e.g. the two idp-claude-sonnet regional entries in litellm.yaml).
  deploymentId?: string;
  // All deployments behind one model alias (the `requested_model` label).
  requestedModel?: string;
}

export function litellmPanels({ deploymentId, requestedModel }: LitellmScope): PanelConfig[] {
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
