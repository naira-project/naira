import { fetchNodes, fetchRelations, NodeResource, RelationResource } from './catalogApi';

/**
 * Discover all unique node kinds from the catalog API.
 * Fetches a large page of nodes and deduplicates their `kind` values.
 */
export async function discoverKinds(): Promise<string[]> {
  const nodes = await fetchNodes({ pageSize: 1000 });
  const kinds = new Set(nodes.map((n) => n.kind).filter(Boolean));
  return Array.from(kinds).sort();
}

/**
 * Infer display columns from a set of nodes of the same kind.
 * Always includes `name` as the first column, then deduplicates
 * all keys found across every node's `props`.
 */
export function inferColumns(nodes: NodeResource[]): string[] {
  const propKeys = new Set<string>();
  for (const node of nodes) {
    if (node.props) {
      for (const key of Object.keys(node.props)) {
        propKeys.add(key);
      }
    }
  }
  return ['name', ...Array.from(propKeys).sort()];
}

/**
 * Format a prop value for table display.
 * - null/undefined → '—'
 * - objects/arrays → JSON-stringified, truncated
 * - everything else → String(value)
 */
export function formatPropValue(value: unknown, maxLen = 60): string {
  if (value === null || value === undefined) {
    return '—';
  }
  if (typeof value === 'object') {
    const json = JSON.stringify(value);
    return json.length > maxLen ? `${json.slice(0, maxLen)}…` : json;
  }
  const str = String(value);
  return str.length > maxLen ? `${str.slice(0, maxLen)}…` : str;
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
 * Check if a prop value is a "complex" type (object or array)
 * that might warrant special rendering in a detail view.
 */
export function isComplexValue(value: unknown): boolean {
  return value !== null && value !== undefined && typeof value === 'object';
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
