import { useState, useEffect } from 'react';
import { computeRelationSummaries, RelationSummary } from '../lib/kindUtils';
import { NodeResource } from '../lib/catalogApi';
import { useOpenMFPContext } from './useOpenMFPContext';

interface UseRelationSummariesResult {
  relationSummaries: Map<string, RelationSummary>;
}

/**
 * Given a list of catalog nodes, computes relation summaries for each node.
 * Re-computes whenever the `nodes` array reference changes.
 */
export function useRelationSummaries(nodes: NodeResource[]): UseRelationSummariesResult {
  const { token } = useOpenMFPContext();
  const [relationSummaries, setRelationSummaries] = useState<Map<string, RelationSummary>>(new Map());

  useEffect(() => {
    if (nodes.length === 0) {
      setRelationSummaries(new Map());
      return;
    }

    computeRelationSummaries(token, nodes)
      .then((summaries) => {
        setRelationSummaries(summaries);
      })
      .catch(() => {
        setRelationSummaries(new Map());
      });
  }, [nodes, token]);

  return { relationSummaries };
}