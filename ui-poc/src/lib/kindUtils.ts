import { fetchNodes, fetchRelations, nodeProps, NodeResource, RelationResource } from './catalogApi';

/**
 * Parse a node path into its structural segments.
 * Path format varies per kind, but the last segment is always the short name,
 * and the second-to-last is the namespace or source.
 *
 * @example
 *   "deployment/8d8c…/cert-manager/cert-manager-cainjector"
 *     → { name: "cert-manager-cainjector", namespace: "cert-manager" }
 *   "model/litellm/idp-claude-sonnet"
 *     → { name: "idp-claude-sonnet", namespace: "litellm" }
 */
export function parsePath(path: string): { name: string; namespace?: string } {
  const segments = path.split('/').filter(Boolean);
  const name = segments[segments.length - 1] ?? path;
  const namespace = segments.length >= 2 ? segments[segments.length - 2] : undefined;
  return { name, namespace };
}

/**
 * Discover all unique node kinds from the catalog API.
 * Fetches a large page of nodes and deduplicates their `kind` values.
 */
// TODO: add API endpoint to fetch kinds directly, instead of fetching nodes and deduplicating
export async function discoverKinds(): Promise<string[]> {
  const nodes = await fetchNodes({ pageSize: 1000 });
  const kinds = new Set(nodes.map((n) => n.kind).filter(Boolean));
  return Array.from(kinds).sort();
}

/**
 * Infer display columns from a set of nodes of the same kind.
 * Always includes `name` and `namespace` as the first columns,
 * then deduplicates all keys found across every node's `props`.
 */
export function inferColumns(nodes: NodeResource[]): string[] {
  const propKeys = new Set<string>();
  for (const node of nodes) {
    const props = nodeProps(node);
    for (const key of Object.keys(props)) {
      propKeys.add(key);
    }
  }
  return ['name', 'namespace', ...Array.from(propKeys).sort()];
}

/**
 * Format a prop value for table display.
 * - null/undefined → '—'
 * - objects/arrays → JSON-stringified, truncated
 * - everything else → String(value)
 */
export function formatPropValue(value: unknown): string {
  if (value === null || value === undefined) return '—';
  if (isComplexValue(value)) {
    const parsed = tryParseJson(value);
    return Array.isArray(parsed)
      ? `[${parsed.length} items]`
      : `{${Object.keys(parsed as object).length} keys}`;
  }
  return String(value);
}

/**
 * Summary of all relations connected to a single node,
 * grouped by relation kind with inbound/outbound counts.
 */
export interface RelationSummary {
  [relationKind: string]: {
    inbound: number;
    outbound: number;
  };
}

/**
 * Safely attempt to parse a value as a JSON string.
 * Returns the parsed object/array if it looks like JSON and parses successfully,
 * otherwise returns the original value.
 */
export function tryParseJson(value: unknown): unknown {
  if (typeof value !== 'string') return value;
  const trimmed = value.trim();
  if (!(trimmed.startsWith('[') || trimmed.startsWith('{'))) return value;
  try {
    return JSON.parse(trimmed);
  } catch {
    return value;
  }
}

/**
 * Determine if a value is complex (an object, an array, or a valid JSON string
 * representing an object or an array).
 */
export function isComplexValue(value: unknown): boolean {
  if (value !== null && typeof value === 'object') return true;
  const parsed = tryParseJson(value);
  return parsed !== value && parsed !== null && typeof parsed === 'object';
}

/**
 * Check if a value is a valid HTTP or HTTPS URL string.
 */
export function isUrlValue(value: unknown): value is string {
  return typeof value === 'string' && /^https?:\/\//i.test(value);
}

/**
 * Compute relation summaries for each node in `nodes`.
 *
 * Fetches all relations from the API, then groups them by relation kind
 * for each node, counting inbound (toNode matches) and outbound (fromNode matches).
 *
 * Returns a Map keyed by node name.
 */
export async function computeRelationSummaries(
  nodes: NodeResource[]
): Promise<Map<string, RelationSummary>> {
  if (nodes.length === 0) {
    return new Map();
  }

  const nodeNames = new Set(nodes.map((n) => n.name));
  const relations = await fetchRelations({ pageSize: 1000 });
  const summaries = new Map<string, RelationSummary>();

  for (const rel of relations) {
    const isInbound = nodeNames.has(rel.toNode);
    const isOutbound = nodeNames.has(rel.fromNode);

    if (!isInbound && !isOutbound) {
      continue;
    }

    if (isInbound) {
      const nodeSummary = summaries.get(rel.toNode) ?? {};
      const kindEntry = nodeSummary[rel.kind] ?? { inbound: 0, outbound: 0 };
      kindEntry.inbound += 1;
      nodeSummary[rel.kind] = kindEntry;
      summaries.set(rel.toNode, nodeSummary);
    }

    if (isOutbound) {
      const nodeSummary = summaries.get(rel.fromNode) ?? {};
      const kindEntry = nodeSummary[rel.kind] ?? { inbound: 0, outbound: 0 };
      kindEntry.outbound += 1;
      nodeSummary[rel.kind] = kindEntry;
      summaries.set(rel.fromNode, nodeSummary);
    }
  }

  return summaries;
}