import { useQuery } from '@tanstack/react-query';
import { computeRelationSummaries, RelationSummary } from '../lib/kindUtils';
import { NodeResource } from '../lib/catalogApi';
import { queryKeys } from '../lib/queryKeys';
import { useOpenMFPContext } from './useOpenMFPContext';

interface UseRelationSummariesResult {
  relationSummaries: Map<string, RelationSummary>;
}

const EMPTY_SUMMARIES = new Map<string, RelationSummary>();

/**
 * Given a list of catalog nodes, computes relation summaries for each node.
 */
export function useRelationSummaries(nodes: NodeResource[]): UseRelationSummariesResult {
  const { token } = useOpenMFPContext();

  const nodeNames = nodes.map((n) => n.name);

  const { data: relationSummaries = EMPTY_SUMMARIES } = useQuery({
    queryKey: queryKeys.relationSummaries(nodeNames),
    queryFn: () => computeRelationSummaries(token, nodes),
    enabled: nodes.length > 0,
  });

  return { relationSummaries };
}
