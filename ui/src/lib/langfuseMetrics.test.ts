import { describe, expect, it } from 'vitest';
import type { CatalogGraphResponse } from './catalogGraph';
import {
  formatCost,
  formatCount,
  formatLatency,
  formatRelativeTime,
  formatScore,
  formatWindow,
  LANGFUSE_PROJECT_KIND,
  langfuseMetricsFromGraph,
} from './langfuseMetrics';

const APP = 'nodes/deployment/naira-idp/default/rag-assistant';

function graph(
  overrides: { nodes?: CatalogGraphResponse['nodes']; edges?: CatalogGraphResponse['edges'] } = {},
): CatalogGraphResponse {
  return { nodes: overrides.nodes ?? [], edges: overrides.edges ?? [] };
}

function metricsNode(path: string, properties: Record<string, string>) {
  return {
    id: `nodes/${LANGFUSE_PROJECT_KIND}/${path}`,
    name: `nodes/${LANGFUSE_PROJECT_KIND}/${path}`,
    kind: LANGFUSE_PROJECT_KIND,
    path,
    label: path,
    depth: 1,
    isRoot: false,
    properties,
  };
}

function observedByEdge(from: string, to: string) {
  return {
    id: `${from}|observed_by|${to}`,
    kind: 'observed_by',
    fromNode: from,
    toNode: to,
    direction: 'outgoing' as const,
  };
}

const fullProps = {
  scope: '',
  traces_url: 'https://langfuse.example.com/project/cm0demo/traces',
  window: '24h0m0s',
  window_start: '2026-09-27T12:00:00Z',
  collected_at: '2026-09-28T12:00:00Z',
  total_cost_usd: '12.34',
  avg_latency: '842',
  latency_unit: 'ms',
  observation_count: '15204',
  score_name: 'user_feedback',
  score_mean: '0.82',
  score_count: '311',
};

describe('langfuseMetricsFromGraph', () => {
  it('reads the scope observing an application', () => {
    const node = metricsNode('demo', fullProps);
    const result = langfuseMetricsFromGraph(
      graph({ nodes: [node], edges: [observedByEdge(APP, node.name)] }),
      APP,
    );

    expect(result).toHaveLength(1);
    expect(result[0]).toMatchObject({
      path: 'demo',
      scope: '',
      tracesUrl: 'https://langfuse.example.com/project/cm0demo/traces',
      totalCostUsd: '12.34',
      avgLatency: '842',
      latencyUnit: 'ms',
      scoreName: 'user_feedback',
      stale: false,
    });
  });

  it('returns nothing when no project observes the application', () => {
    expect(langfuseMetricsFromGraph(graph(), APP)).toEqual([]);
  });

  it('ignores projects observing a different application', () => {
    const node = metricsNode('demo', fullProps);
    const result = langfuseMetricsFromGraph(
      graph({
        nodes: [node],
        edges: [observedByEdge('nodes/deployment/other', node.name)],
      }),
      APP,
    );

    expect(result).toEqual([]);
  });

  it('ignores edges of another kind', () => {
    const node = metricsNode('demo', fullProps);
    const result = langfuseMetricsFromGraph(
      graph({
        nodes: [node],
        edges: [{ ...observedByEdge(APP, node.name), kind: 'uses_model' }],
      }),
      APP,
    );

    expect(result).toEqual([]);
  });

  // A project split by trace name gives each application its own slice.
  it('returns every scope bound to the application, ordered by path', () => {
    const escalation = metricsNode('support/escalation-agent', fullProps);
    const triage = metricsNode('support/triage-agent', fullProps);

    const result = langfuseMetricsFromGraph(
      graph({
        nodes: [triage, escalation],
        edges: [observedByEdge(APP, triage.name), observedByEdge(APP, escalation.name)],
      }),
      APP,
    );

    expect(result.map((scope) => scope.path)).toEqual([
      'support/escalation-agent',
      'support/triage-agent',
    ]);
  });

  it('reads the stale markers a degraded sync writes', () => {
    const node = metricsNode('demo', {
      ...fullProps,
      stale: 'true',
      stale_since: '2026-09-28T10:00:00Z',
      last_error: 'langfuse /api/public/v2/metrics returned 502 Bad Gateway',
    });

    const [scope] = langfuseMetricsFromGraph(
      graph({ nodes: [node], edges: [observedByEdge(APP, node.name)] }),
      APP,
    );

    expect(scope.stale).toBe(true);
    expect(scope.staleSince).toBe('2026-09-28T10:00:00Z');
    expect(scope.lastError).toContain('502');
  });

  it('tolerates a node with no properties', () => {
    const node = { ...metricsNode('demo', {}), properties: undefined };
    const [scope] = langfuseMetricsFromGraph(
      graph({ nodes: [node], edges: [observedByEdge(APP, node.name)] }),
      APP,
    );

    expect(scope.totalCostUsd).toBe('');
    expect(scope.stale).toBe(false);
  });
});

describe('formatCost', () => {
  it('formats ordinary spend as currency', () => {
    expect(formatCost('12.3456')).toBe('$12.35');
    expect(formatCost('1234.5')).toBe('$1,234.50');
  });

  it('distinguishes a measured zero from sub-cent spend', () => {
    expect(formatCost('0')).toBe('$0.00');
    expect(formatCost('0.000000123')).toBe('<$0.01');
  });

  // An empty value means the window was never measured, not that it cost zero.
  it('reports no data rather than zero', () => {
    expect(formatCost('')).toBeNull();
    expect(formatCost('   ')).toBeNull();
    expect(formatCost('not-a-number')).toBeNull();
  });

  it('abbreviates very large totals', () => {
    expect(formatCost('2500000')).toBe('$2.5M');
  });
});

describe('formatLatency', () => {
  it('formats milliseconds', () => {
    expect(formatLatency('842', 'ms')).toBe('842 ms');
    expect(formatLatency('842.6', 'ms')).toBe('843 ms');
  });

  it('promotes long latencies to seconds', () => {
    expect(formatLatency('1500', 'ms')).toBe('1.5 s');
    expect(formatLatency('45000', 'ms')).toBe('45 s');
  });

  it('converts a seconds-based unit', () => {
    expect(formatLatency('0.842', 's')).toBe('842 ms');
  });

  // The plugin's unit is the least certain part of its schema, so an
  // unrecognised one is shown as-is rather than guessed at.
  it('passes an unknown unit through', () => {
    expect(formatLatency('1.5', 'ns')).toBe('1.5 ns');
    expect(formatLatency('1.5', '')).toBe('1.5');
  });

  it('reports no data for an empty value', () => {
    expect(formatLatency('', 'ms')).toBeNull();
  });
});

describe('formatCount', () => {
  it('groups thousands', () => {
    expect(formatCount('15204')).toBe('15,204');
  });

  it('abbreviates millions', () => {
    expect(formatCount('2500000')).toBe('2.5M');
  });

  it('reports no data for an empty value', () => {
    expect(formatCount('')).toBeNull();
  });
});

describe('formatScore', () => {
  // Scores are not assumed to be ratios: 4.5 out of 5 must stay 4.5.
  it('renders the score as its own number', () => {
    expect(formatScore('0.82')).toBe('0.82');
    expect(formatScore('4.5')).toBe('4.5');
    expect(formatScore('0.8231')).toBe('0.82');
  });

  it('reports no data for an empty value', () => {
    expect(formatScore('')).toBeNull();
  });
});

describe('formatRelativeTime', () => {
  const now = new Date('2026-09-28T12:00:00Z');

  it('describes recent instants', () => {
    expect(formatRelativeTime('2026-09-28T11:59:30Z', now)).toBe('just now');
    expect(formatRelativeTime('2026-09-28T11:48:00Z', now)).toBe('12 min ago');
    expect(formatRelativeTime('2026-09-28T09:00:00Z', now)).toBe('3 h ago');
    expect(formatRelativeTime('2026-09-26T12:00:00Z', now)).toBe('2 d ago');
  });

  it('returns null for an absent or unparseable timestamp', () => {
    expect(formatRelativeTime('', now)).toBeNull();
    expect(formatRelativeTime('nonsense', now)).toBeNull();
  });
});

describe('formatWindow', () => {
  it('reads Go duration strings', () => {
    expect(formatWindow('24h0m0s')).toBe('last 24h');
    expect(formatWindow('168h0m0s')).toBe('last 7d');
    expect(formatWindow('6h0m0s')).toBe('last 6h');
  });

  it('passes anything else through', () => {
    expect(formatWindow('90m0s')).toBe('last 90m0s');
    expect(formatWindow('')).toBeNull();
  });
});
