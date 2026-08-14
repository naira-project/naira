import { useQuery } from '@tanstack/react-query';
import { computeRelationSummaries, RelationSummary } from '../lib/kindUtils';
import { NodeResource } from '../lib/catalogApi';
import { queryKeys } from '../lib/queryKeys';

interface UseRelationSummariesResult {
  relationSummaries: Map<string, RelationSummary>;
}

const EMPTY_SUMMARIES = new Map<string, RelationSummary>();

/**
 * Given a list of catalog nodes, computes relation summaries for each node.
 * Keyed by the (sorted) set of node names, so the same set of nodes always
 * hits the same cache entry regardless of array identity.
 */
export function useRelationSummaries(nodes: NodeResource[]): UseRelationSummariesResult {
  const nodeNames = nodes.map((n) => n.name);

  const { data: relationSummaries = EMPTY_SUMMARIES } = useQuery({
    queryKey: queryKeys.relationSummaries(nodeNames),
    queryFn: () => computeRelationSummaries(nodes),
    enabled: nodes.length > 0,
  });

  return { relationSummaries };
}
