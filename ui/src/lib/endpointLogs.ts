import { type NodeResource, nodeProps } from './catalogApi';
import type { LogsConfig } from './runtimeConfig';

// In-cluster Service hosts: llama-qwen25-05b.test-dependencies.svc.cluster.local
// or just llama-qwen25-05b, which resolves in the caller's own namespace.
const CLUSTER_SERVICE_HOST = /^([a-z0-9-]+)(?:\.([a-z0-9-]+)\.svc(?:\.cluster\.local)?)?$/;

// Fills {{key}} placeholders. Undefined when a placeholder has no value, so a
// query never silently drops a matcher.
function fillTemplate(template: string, values: Record<string, string>): string | undefined {
  let complete = true;
  const query = template.replace(/\{\{(\w+)\}\}/g, (_, key: string) => {
    if (!values[key]) {
      complete = false;
    }
    return values[key] ?? '';
  });
  return complete ? query : undefined;
}

/**
 * LogQL queries selecting an inference endpoint's logs:
 * - the backend pods, when api_base points at an in-cluster Service, and
 * - the proxy's lines mentioning the upstream model.
 *
 * Endpoints without either, such as those from the Bedrock plugin whose logs
 * live in CloudWatch, get no queries.
 */
export function endpointLogQueries(node: NodeResource, logs: LogsConfig): string[] {
  if (node.kind !== 'inference_endpoint') {
    return [];
  }
  const props = nodeProps(node);
  const namespace = logs.defaultNamespace ?? '';
  const queries: (string | undefined)[] = [];

  const service = clusterService(props.api_base, namespace);
  if (service) {
    queries.push(fillTemplate(logs.serviceQuery, service));
  }

  const model = upstreamModelName(props.model);
  if (model) {
    queries.push(fillTemplate(logs.proxyQuery, { namespace, model: JSON.stringify(model) }));
  }

  return queries.filter((q): q is string => q !== undefined);
}

/** Explore URL showing the endpoint's logs, or undefined when there are none to show. */
export function endpointLogsUrl(node: NodeResource, logs?: LogsConfig): string | undefined {
  if (!logs) {
    return undefined;
  }
  const queries = endpointLogQueries(node, logs);
  if (queries.length === 0) {
    return undefined;
  }

  const pane = {
    datasource: logs.datasource.uid,
    queries: queries.map((expr, i) => ({
      refId: String.fromCharCode('A'.charCodeAt(0) + i),
      datasource: logs.datasource,
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
  return `${logs.explorerUrl.replace(/\/+$/, '')}/explore?${params}`;
}

function clusterService(
  apiBase: string | undefined,
  defaultNamespace: string,
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
  const namespace = match?.[2] ?? defaultNamespace;
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
