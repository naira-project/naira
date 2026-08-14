/**
 * Centralized query key factory. Every hook that reads or invalidates
 * server state should build its key(s) from here, so keys never drift
 * out of sync between the place that fetches and the place that
 * invalidates.
 */
export const queryKeys = {
  kinds: ['kinds'] as const,

  nodes: (kind: string) => ['nodes', kind] as const,
  node: (kind: string, path: string) => ['node', kind, path] as const,
  nodeByName: (name: string) => ['nodeByName', name] as const,

  relatedNodes: (nodeName: string, relationKind?: string) =>
    ['relatedNodes', nodeName, relationKind ?? null] as const,

  relationSummaries: (nodeNames: string[]) =>
    ['relationSummaries', [...nodeNames].sort()] as const,

  graph: (rootName: string | null, depth: number) => ['graph', rootName, depth] as const,

  plugins: ['plugins'] as const,
  operations: ['operations'] as const,
  operation: (name: string) => ['operation', name] as const,
};
