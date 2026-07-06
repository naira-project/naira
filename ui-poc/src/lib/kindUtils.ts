import { fetchNodes, NodeResource } from './catalogApi';

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
 * Check if a prop value is a "complex" type (object or array)
 * that might warrant special rendering in a detail view.
 */
export function isComplexValue(value: unknown): boolean {
  return value !== null && value !== undefined && typeof value === 'object';
}
