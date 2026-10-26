import { type NodeResource, nodeProps } from './catalogApi';

// Must match the Loki entry in grafana.additionalDataSources (uid: loki) in the
// test-dependencies chart's values.yaml.
const LOKI_DATASOURCE = { type: 'loki', uid: 'loki' };

// Port-forward of the test-dependencies Grafana service.
const DEFAULT_GRAFANA_URL = 'http://localhost:3002';

function grafanaUrl(): string {
  return (import.meta.env.VITE_GRAFANA_URL || DEFAULT_GRAFANA_URL).replace(/\/+$/, '');
}

// Namespace the workloads run in, set at build time. Used for the LiteLLM proxy
// and for Services whose host carries no namespace. Empty -> any namespace.
function defaultNamespace(): string {
  return import.meta.env.VITE_NAMESPACE ?? '';
}

// The `app` label is the chart name for LiteLLM (e.g. litellm-helm), hence the prefix match.
function litellmProxySelector(): string {
  const namespace = defaultNamespace();
  return namespace ? `{namespace="${namespace}", app=~"litellm.*"}` : '{app=~"litellm.*"}';
}

// In-cluster Service hosts: llama-qwen25-05b.test-dependencies.svc.cluster.local
// or just llama-qwen25-05b, which resolves in the caller's own namespace.
const CLUSTER_SERVICE_HOST = /^([a-z0-9-]+)(?:\.([a-z0-9-]+)\.svc(?:\.cluster\.local)?)?$/;

/**
 * LogQL queries selecting an inference endpoint's logs in Loki:
 * - the backend pods, when api_base points at an in-cluster Service (the
 *   test-dependencies Alloy labels pods by namespace and their `app` label,
 *   which the chart sets to the Service name), and
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

  const model = upstreamModelName(props.model);
  if (model) {
    queries.push(`${litellmProxySelector()} |= ${JSON.stringify(model)}`);
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
  return `${grafanaUrl()}/explore?${params}`;
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
  const namespace = match?.[2] ?? defaultNamespace();
  return match && namespace ? { name: match[1], namespace } : undefined;
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
