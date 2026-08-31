/**
 * Centralized query key factory. Every hook that reads or invalidates
 * server state should build its key(s) from here, so keys never drift
 * out of sync between the place that fetches and the place that
 * invalidates.
 */
export const queryKeys = {
  catalog: ['catalog'] as const,
  kinds: ['catalog', 'kinds'] as const,

  nodes: (kind: string) => ['catalog', 'nodes', kind] as const,
  node: (kind: string, path: string) => ['catalog', 'node', kind, path] as const,

  relationSummaries: (nodeNames: string[]) =>
    ['catalog', 'relationSummaries', [...nodeNames].sort()] as const,

  graph: (rootName: string | null, depth: number) => ['catalog', 'graph', rootName, depth] as const,

  plugins: ['plugins'] as const,
  operations: ['operations'] as const,
};
