import { describe, expect, it } from 'vitest';
import type { NodeResource } from './catalogApi';
import { endpointLogQueries, endpointLogsUrl } from './endpointLogs';

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
      upstream_model: 'openai/qwen2.5-0.5b-instruct',
    });

    expect(endpointLogQueries(node)).toEqual([
      '{namespace="llamacpp", app="llama-qwen25-05b"}',
      '{namespace="litellm", app="litellm"} |= "qwen2.5-0.5b-instruct"',
    ]);
  });

  it('selects only the proxy lines for an external endpoint', () => {
    const node = endpointNode('litellm', 'litellm/idp-gpt-4o-mini-def456', {
      api_base: 'https://api.openai.com/v1',
      upstream_model: 'openai/gpt-4o-mini',
    });

    expect(endpointLogQueries(node)).toEqual([
      '{namespace="litellm", app="litellm"} |= "gpt-4o-mini"',
    ]);
  });

  it('keeps nested provider segments after the routing prefix', () => {
    const node = endpointNode('litellm', 'litellm/openrouter-gpt-3.5-turbo-ghi789', {
      api_base: 'https://openrouter.ai/api/v1',
      upstream_model: 'openrouter/openai/gpt-3.5-turbo',
    });

    expect(endpointLogQueries(node)).toEqual([
      '{namespace="litellm", app="litellm"} |= "openai/gpt-3.5-turbo"',
    ]);
  });

  it('returns nothing for a Bedrock plugin endpoint', () => {
    const node = endpointNode('bedrock', 'bedrock/amazon.nova-micro-v1:0-us-east-1', {
      provider: 'bedrock',
      region: 'us-east-1',
      model_id: 'amazon.nova-micro-v1:0',
    });

    expect(endpointLogQueries(node)).toEqual([]);
    expect(endpointLogsUrl(node)).toBeUndefined();
  });

  it('returns nothing for other kinds', () => {
    const node: NodeResource = {
      name: 'nodes/model/litellm/idp-gpt-4o-mini',
      kind: 'model',
      path: 'litellm/idp-gpt-4o-mini',
      pluginClaims: [{ plugin: 'litellm', props: { upstream_model: 'openai/gpt-4o-mini' } }],
    };

    expect(endpointLogQueries(node)).toEqual([]);
  });
});

describe('endpointLogsUrl', () => {
  it('opens Grafana Explore on the Loki datasource with one query per source', () => {
    const node = endpointNode('litellm', 'litellm/idp-llama-qwen25-05b-abc123', {
      api_base: 'http://llama-qwen25-05b.llamacpp.svc.cluster.local:8080/v1',
      upstream_model: 'openai/qwen2.5-0.5b-instruct',
    });

    const url = new URL(endpointLogsUrl(node) ?? '');

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
            expr: '{namespace="litellm", app="litellm"} |= "qwen2.5-0.5b-instruct"',
          },
        ],
        range: { from: 'now-1h', to: 'now' },
      },
    });
  });
});
