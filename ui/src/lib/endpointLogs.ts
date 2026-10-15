import { type NodeResource, nodeProps } from './catalogApi';

// Must match grafana.additionalDataSources[].uid in
// deploy/dev/stacks/llm-inference/infra/helm/prometheus-values.yaml.
const LOKI_DATASOURCE = { type: 'loki', uid: 'loki' };

// Defaults to the port-forward opened by `task forward:llm-inference`.
const GRAFANA_URL = (import.meta.env.VITE_GRAFANA_URL ?? 'http://localhost:3002').replace(
  /\/+$/,
  '',
);

// Every LiteLLM deployment is called through this proxy, so its logs are the
// one source shared by internal and external endpoints.
const LITELLM_PROXY_SELECTOR = '{namespace="litellm", app="litellm"}';

// In-cluster Service hosts, e.g. llama-qwen25-05b.llamacpp.svc.cluster.local.
const CLUSTER_SERVICE_HOST = /^([a-z0-9-]+)\.([a-z0-9-]+)\.svc(\.cluster\.local)?$/;

/**
 * LogQL queries selecting an inference endpoint's logs in Loki:
 * - the backend pods, when api_base points at an in-cluster Service (Alloy
 *   labels pods by namespace and `app`, which the dev manifests set to the
 *   Service name), and
 * - the LiteLLM proxy's lines mentioning the upstream model.
 *
 * Endpoints without either, such as those from the Bedrock plugin whose logs
 * live in CloudWatch, get no queries.
 */
export function endpointLogQueries(node: NodeResource): string[] {
  if (node.kind !== 'inference_endpoint') {
    return [];
  }
  const props = nodeProps(node);
  const queries: string[] = [];

  const service = clusterService(props.api_base);
  if (service) {
    queries.push(`{namespace="${service.namespace}", app="${service.name}"}`);
  }

  const model = upstreamModelName(props.upstream_model);
  if (model) {
    queries.push(`${LITELLM_PROXY_SELECTOR} |= ${JSON.stringify(model)}`);
  }

  return queries;
}

/** Grafana Explore URL showing the endpoint's logs, or undefined when there are none to show. */
export function endpointLogsUrl(node: NodeResource): string | undefined {
  const queries = endpointLogQueries(node);
  if (queries.length === 0) {
    return undefined;
  }

  const pane = {
    datasource: LOKI_DATASOURCE.uid,
    queries: queries.map((expr, i) => ({
      refId: String.fromCharCode('A'.charCodeAt(0) + i),
      datasource: LOKI_DATASOURCE,
      queryType: 'range',
      expr,
    })),
    range: { from: 'now-1h', to: 'now' },
  };
  const params = new URLSearchParams({
    schemaVersion: '1',
    orgId: '1',
    panes: JSON.stringify({ a: pane }),
  });
  return `${GRAFANA_URL}/explore?${params}`;
}

function clusterService(
  apiBase: string | undefined,
): { name: string; namespace: string } | undefined {
  if (!apiBase) {
    return undefined;
  }
  let host: string;
  try {
    host = new URL(apiBase).hostname;
  } catch {
    return undefined;
  }
  const match = CLUSTER_SERVICE_HOST.exec(host);
  return match ? { name: match[1], namespace: match[2] } : undefined;
}

// LiteLLM logs the model without its routing prefix, e.g. "gpt-4o-mini" for
// "openai/gpt-4o-mini" and "openai/gpt-3.5-turbo" for "openrouter/openai/gpt-3.5-turbo".
function upstreamModelName(upstreamModel: string | undefined): string | undefined {
  const model = upstreamModel?.trim();
  if (!model) {
    return undefined;
  }
  const slash = model.indexOf('/');
  return slash === -1 ? model : model.slice(slash + 1);
}
