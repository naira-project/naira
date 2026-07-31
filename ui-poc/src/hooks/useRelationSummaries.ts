import { useState, useEffect } from 'react';
import { computeRelationSummaries, RelationSummary } from '../lib/kindUtils';
import { NodeResource } from '../lib/catalogApi';

interface UseRelationSummariesResult {
  relationSummaries: Map<string, RelationSummary>;
}

/**
 * Given a list of catalog nodes, computes relation summaries for each node.
 * Re-computes whenever the `nodes` array reference changes.
 */
export function useRelationSummaries(nodes: NodeResource[]): UseRelationSummariesResult {
  const [relationSummaries, setRelationSummaries] = useState<Map<string, RelationSummary>>(new Map());

  useEffect(() => {
    if (nodes.length === 0) {
      setRelationSummaries(new Map());
      return;
    }

    computeRelationSummaries(nodes)
      .then((summaries) => {
        setRelationSummaries(summaries);
      })
      .catch(() => {
        setRelationSummaries(new Map());
      });
  }, [nodes]);

  return { relationSummaries };
}