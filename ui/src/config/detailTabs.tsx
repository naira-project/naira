import { BrainCircuit, Cloud, Wrench } from 'lucide-react';
import type { ComponentType } from 'react';
import LangfuseMetrics from '../components/LangfuseMetrics';
import type { RelatedNodesConfig } from '../components/RelatedNodes';
import type { NodeResource } from '../lib/catalogApi';

export const RELATED_CARDS_BY_KIND: Record<string, RelatedNodesConfig> = {
  mcp_server: {
    relationKind: 'exposes',
    direction: 'outgoing',
    title: 'tools',
    icon: Wrench,
    countSuffix: 'exposed',
    emptyText: (
      <>
        This server exposes no tools. If it is unreachable its tools cannot be read — check the{' '}
        <span className="font-medium">reachable</span> property.
      </>
    ),
  },
  inference_endpoint: {
    relationKind: 'serves_model',
    direction: 'outgoing',
    title: 'Uses Model',
    icon: BrainCircuit,
    description: 'The model this inference endpoint serves.',
  },
  model: {
    relationKind: 'serves_model',
    direction: 'incoming',
    title: 'Served By',
    icon: Cloud,
    description: 'The inference endpoint that serves this model.',
    emptyText:
      'No inference endpoint serves this model. Endpoints only appear once the model has received traffic within the metrics lookback window.',
  },
};

export function relatedCardForKind(kind: string): RelatedNodesConfig | undefined {
  return RELATED_CARDS_BY_KIND[kind];
}

/** Kinds that can carry LLM observability; the panel is silent without a bound Langfuse project. */
const LANGFUSE_OBSERVABLE_KINDS = ['deployment', 'application'];

/**
 * Extra panels appended to a kind's Properties tab. A related-nodes card suits
 * neighbours worth linking to; Langfuse metrics need rendering, not a link.
 */
export const PROPERTY_PANELS_BY_KIND: Record<string, ComponentType<{ node: NodeResource }>> =
  Object.fromEntries(LANGFUSE_OBSERVABLE_KINDS.map((kind) => [kind, LangfuseMetrics]));

export function propertyPanelForKind(
  kind: string,
): ComponentType<{ node: NodeResource }> | undefined {
  return PROPERTY_PANELS_BY_KIND[kind];
}
