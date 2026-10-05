import type { CatalogGraphResponse } from './catalogGraph';

export const LANGFUSE_PROJECT_KIND = 'langfuse_project';
export const LANGFUSE_PLUGIN = 'langfuse';
const OBSERVED_BY_RELATION = 'observed_by';

/**
 * One Langfuse scope observing an application. Metrics are raw strings as the
 * plugin emitted them: the catalog stores properties as strings, and an empty
 * one means "not measured" rather than zero. The format* helpers below handle
 * that distinction, returning null for absent data.
 */
export interface LangfuseMetrics {
  /** Catalog resource name of the langfuse_project node. */
  name: string;
  kind: string;
  path: string;
  /** Trace name this slice covers; empty means the project as a whole. */
  scope: string;
  tracesUrl: string;
  window: string;
  collectedAt: string;
  stale: boolean;
  staleSince: string;
  lastError: string;
  totalCostUsd: string;
  avgLatency: string;
  latencyUnit: string;
  observationCount: string;
  scoreName: string;
  scoreMean: string;
  scoreCount: string;
}

function propString(properties: Record<string, unknown> | undefined, key: string): string {
  const value = properties?.[key];
  return typeof value === 'string' ? value : '';
}

/**
 * The Langfuse scopes observing an application, read from its depth-1 graph
 * slice: the `observed_by` edges leaving the app, resolved against the
 * neighbour nodes. Several when the project is split by trace name, none when
 * nothing is bound.
 */
export function langfuseMetricsFromGraph(
  graph: CatalogGraphResponse,
  appName: string,
): LangfuseMetrics[] {
  const observed = new Set(
    graph.edges
      .filter((edge) => edge.kind === OBSERVED_BY_RELATION && edge.fromNode === appName)
      .map((edge) => edge.toNode),
  );

  return graph.nodes
    .filter((node) => node.kind === LANGFUSE_PROJECT_KIND && observed.has(node.name))
    .map((node): LangfuseMetrics => {
      const props = node.properties;
      return {
        name: node.name,
        kind: node.kind,
        path: node.path,
        scope: propString(props, 'scope'),
        tracesUrl: propString(props, 'traces_url'),
        window: propString(props, 'window'),
        collectedAt: propString(props, 'collected_at'),
        stale: propString(props, 'stale') === 'true',
        staleSince: propString(props, 'stale_since'),
        lastError: propString(props, 'last_error'),
        totalCostUsd: propString(props, 'total_cost_usd'),
        avgLatency: propString(props, 'avg_latency'),
        latencyUnit: propString(props, 'latency_unit'),
        observationCount: propString(props, 'observation_count'),
        scoreName: propString(props, 'score_name'),
        scoreMean: propString(props, 'score_mean'),
        scoreCount: propString(props, 'score_count'),
      };
    })
    .sort((a, b) => a.path.localeCompare(b.path));
}

/**
 * Returns null for anything not a finite number, including the empty string
 * the plugin writes for an unmeasured window — which tiles must show as "no
 * data", not as a confident zero.
 */
function toNumber(raw: string): number | null {
  if (raw.trim() === '') {
    return null;
  }
  const value = Number(raw);
  return Number.isFinite(value) ? value : null;
}

/** Threshold above which values are abbreviated to keep a tile narrow. */
const COMPACT_FROM = 1_000_000;

export function formatCost(raw: string): string | null {
  const value = toNumber(raw);
  if (value === null) {
    return null;
  }
  if (value >= COMPACT_FROM) {
    return value.toLocaleString('en-US', {
      style: 'currency',
      currency: 'USD',
      notation: 'compact',
      maximumFractionDigits: 1,
    });
  }
  // Sub-cent spend is real; "under a cent" beats a rounded $0.00.
  if (value > 0 && value < 0.01) {
    return '<$0.01';
  }
  return value.toLocaleString('en-US', {
    style: 'currency',
    currency: 'USD',
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
}

/**
 * Formats a latency in the unit the plugin labelled it with. An unrecognised
 * unit is passed through rather than guessed at.
 */
export function formatLatency(raw: string, unit: string): string | null {
  const value = toNumber(raw);
  if (value === null) {
    return null;
  }

  const milliseconds = unit === 'ms' ? value : unit === 's' ? value * 1000 : null;
  if (milliseconds === null) {
    const formatted = value.toLocaleString('en-US', { maximumFractionDigits: 2 });
    return unit ? `${formatted} ${unit}` : formatted;
  }

  if (milliseconds < 1000) {
    return `${Math.round(milliseconds)} ms`;
  }
  const seconds = milliseconds / 1000;
  return `${seconds.toLocaleString('en-US', { maximumFractionDigits: seconds < 10 ? 2 : 1 })} s`;
}

export function formatCount(raw: string): string | null {
  const value = toNumber(raw);
  if (value === null) {
    return null;
  }
  if (value >= COMPACT_FROM) {
    return value.toLocaleString('en-US', { notation: 'compact', maximumFractionDigits: 1 });
  }
  return value.toLocaleString('en-US', { maximumFractionDigits: 0 });
}

/**
 * A plain number, deliberately: a score may be 1–5 stars as easily as a
 * thumbs-up ratio, so rendering it as a percentage would invent meaning.
 */
export function formatScore(raw: string): string | null {
  const value = toNumber(raw);
  if (value === null) {
    return null;
  }
  return value.toLocaleString('en-US', { maximumFractionDigits: 2 });
}

/**
 * How long ago an instant was, for the freshness line. Null for an absent or
 * unparseable timestamp, so the caller can omit the line entirely.
 */
export function formatRelativeTime(iso: string, now: Date = new Date()): string | null {
  if (iso.trim() === '') {
    return null;
  }
  const then = new Date(iso);
  if (Number.isNaN(then.getTime())) {
    return null;
  }

  const seconds = Math.round((now.getTime() - then.getTime()) / 1000);
  if (seconds < 0) {
    return 'just now';
  }
  if (seconds < 60) {
    return 'just now';
  }

  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) {
    return `${minutes} min ago`;
  }

  const hours = Math.floor(minutes / 60);
  if (hours < 24) {
    return `${hours} h ago`;
  }

  const days = Math.floor(hours / 24);
  return `${days} d ago`;
}

/** Readable form of the plugin's Go duration, which renders a day "24h0m0s". */
export function formatWindow(raw: string): string | null {
  const match = raw.trim().match(/^(\d+)h0m0s$/);
  if (match) {
    const hours = Number(match[1]);
    if (hours % 24 === 0 && hours >= 24) {
      const days = hours / 24;
      return days === 1 ? 'last 24h' : `last ${days}d`;
    }
    return `last ${hours}h`;
  }
  return raw.trim() === '' ? null : `last ${raw.trim()}`;
}
