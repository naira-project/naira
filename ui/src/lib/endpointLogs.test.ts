import { describe, expect, it } from 'vitest';
import type { NodeResource } from './catalogApi';
import { endpointLogQueries, endpointLogsUrl } from './endpointLogs';
import type { LogsConfig } from './runtimeConfig';

const logsConfig: LogsConfig = {
  explorerUrl: 'http://localhost:3002',
  datasource: { type: 'loki', uid: 'loki' },
  serviceQuery: '{namespace="{{namespace}}", app="{{name}}"}',
  proxyQuery: '{app=~"litellm.*"} |= {{model}}',
};

function endpointNode(plugin: string, path: string, props: Record<string, string>): NodeResource {
  return {
    name: `nodes/inference_endpoint/${path}`,
    kind: 'inference_endpoint',
    path,
    pluginClaims: [{ plugin, props }],
  };
}

describe('endpointLogQueries', () => {
  it('selects the backend pods and the proxy lines for an in-cluster endpoint', () => {
    const node = endpointNode('litellm', 'litellm/idp-llama-qwen25-05b-abc123', {
      api_base: 'http://llama-qwen25-05b.llamacpp.svc.cluster.local:8080/v1',
      model: 'openai/qwen2.5-0.5b-instruct',
    });

    expect(endpointLogQueries(node, logsConfig)).toEqual([
      '{namespace="llamacpp", app="llama-qwen25-05b"}',
      '{app=~"litellm.*"} |= "qwen2.5-0.5b-instruct"',
    ]);
  });

  it('selects only the proxy lines for an external endpoint', () => {
    const node = endpointNode('litellm', 'litellm/idp-gpt-4o-mini-def456', {
      api_base: 'https://api.openai.com/v1',
      model: 'openai/gpt-4o-mini',
    });

    expect(endpointLogQueries(node, logsConfig)).toEqual(['{app=~"litellm.*"} |= "gpt-4o-mini"']);
  });

  it('keeps nested provider segments after the routing prefix', () => {
    const node = endpointNode('litellm', 'litellm/openrouter-gpt-3.5-turbo-ghi789', {
      api_base: 'https://openrouter.ai/api/v1',
      model: 'openrouter/openai/gpt-3.5-turbo',
    });

    expect(endpointLogQueries(node, logsConfig)).toEqual([
      '{app=~"litellm.*"} |= "openai/gpt-3.5-turbo"',
    ]);
  });

  it('returns nothing for a Bedrock plugin endpoint', () => {
    const node = endpointNode('bedrock', 'bedrock/amazon.nova-micro-v1:0-us-east-1', {
      provider: 'bedrock',
      region: 'us-east-1',
      model_id: 'amazon.nova-micro-v1:0',
    });

    expect(endpointLogQueries(node, logsConfig)).toEqual([]);
    expect(endpointLogsUrl(node, logsConfig)).toBeUndefined();
  });

  it('returns nothing for other kinds', () => {
    const node: NodeResource = {
      name: 'nodes/model/litellm/idp-gpt-4o-mini',
      kind: 'model',
      path: 'litellm/idp-gpt-4o-mini',
      pluginClaims: [{ plugin: 'litellm', props: { model: 'openai/gpt-4o-mini' } }],
    };

    expect(endpointLogQueries(node, logsConfig)).toEqual([]);
  });
});

describe('runtime config', () => {
  it('fills the configured namespace into the proxy query', () => {
    const node = endpointNode('litellm', 'litellm/idp-gpt-4o-mini-def456', {
      api_base: 'https://api.openai.com/v1',
      model: 'openai/gpt-4o-mini',
    });
    const config: LogsConfig = {
      ...logsConfig,
      defaultNamespace: 'naira-deps',
      proxyQuery: '{namespace="{{namespace}}", app=~"litellm.*"} |= {{model}}',
    };

    expect(endpointLogQueries(node, config)).toEqual([
      '{namespace="naira-deps", app=~"litellm.*"} |= "gpt-4o-mini"',
    ]);
  });

  it('places a Service host without a namespace in the configured namespace', () => {
    const node = endpointNode('litellm', 'litellm/idp-llama-qwen25-05b-abc123', {
      api_base: 'http://llama-qwen25-05b:8080/v1',
      model: 'openai/qwen2.5-0.5b-instruct',
    });
    const config: LogsConfig = { ...logsConfig, defaultNamespace: 'naira-deps' };

    expect(endpointLogQueries(node, config)).toEqual([
      '{namespace="naira-deps", app="llama-qwen25-05b"}',
      '{app=~"litellm.*"} |= "qwen2.5-0.5b-instruct"',
    ]);
  });

  it('ignores a Service host without a namespace when none is configured', () => {
    const node = endpointNode('litellm', 'litellm/idp-llama-qwen25-05b-abc123', {
      api_base: 'http://llama-qwen25-05b:8080/v1',
      model: 'openai/qwen2.5-0.5b-instruct',
    });

    expect(endpointLogQueries(node, logsConfig)).toEqual([
      '{app=~"litellm.*"} |= "qwen2.5-0.5b-instruct"',
    ]);
  });

  it('skips a query whose placeholder has no value', () => {
    const node = endpointNode('litellm', 'litellm/idp-gpt-4o-mini-def456', {
      api_base: 'https://api.openai.com/v1',
      model: 'openai/gpt-4o-mini',
    });
    const config: LogsConfig = {
      ...logsConfig,
      proxyQuery: '{namespace="{{namespace}}", app=~"litellm.*"} |= {{model}}',
    };

    expect(endpointLogQueries(node, config)).toEqual([]);
  });

  it('links to the configured explorer without a trailing slash', () => {
    const node = endpointNode('litellm', 'litellm/idp-gpt-4o-mini-def456', {
      api_base: 'https://api.openai.com/v1',
      model: 'openai/gpt-4o-mini',
    });
    const config: LogsConfig = { ...logsConfig, explorerUrl: 'https://grafana.example.com/' };

    const url = new URL(endpointLogsUrl(node, config) ?? '');

    expect(`${url.origin}${url.pathname}`).toBe('https://grafana.example.com/explore');
  });

  it('offers no link when no logs are configured', () => {
    const node = endpointNode('litellm', 'litellm/idp-gpt-4o-mini-def456', {
      api_base: 'https://api.openai.com/v1',
      model: 'openai/gpt-4o-mini',
    });

    expect(endpointLogsUrl(node, undefined)).toBeUndefined();
  });
});

describe('endpointLogsUrl', () => {
  it('opens Explore on the configured datasource with one query per source', () => {
    const node = endpointNode('litellm', 'litellm/idp-llama-qwen25-05b-abc123', {
      api_base: 'http://llama-qwen25-05b.llamacpp.svc.cluster.local:8080/v1',
      model: 'openai/qwen2.5-0.5b-instruct',
    });

    const url = new URL(endpointLogsUrl(node, logsConfig) ?? '');

    expect(`${url.origin}${url.pathname}`).toBe('http://localhost:3002/explore');
    expect(url.searchParams.get('orgId')).toBe('1');
    expect(JSON.parse(url.searchParams.get('panes') ?? '')).toEqual({
      a: {
        datasource: 'loki',
        queries: [
          {
            refId: 'A',
            datasource: { type: 'loki', uid: 'loki' },
            queryType: 'range',
            expr: '{namespace="llamacpp", app="llama-qwen25-05b"}',
          },
          {
            refId: 'B',
            datasource: { type: 'loki', uid: 'loki' },
            queryType: 'range',
            expr: '{app=~"litellm.*"} |= "qwen2.5-0.5b-instruct"',
          },
        ],
        range: { from: 'now-1h', to: 'now' },
      },
    });
  });
});
