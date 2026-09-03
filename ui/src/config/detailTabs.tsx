import { Wrench, BrainCircuit, Cloud } from 'lucide-react';
import type { RelatedNodesConfig } from '../components/RelatedNodes';

export const MCP_SERVER_KIND = 'mcp_server';
const RELATION_KIND_SERVES_MODEL = 'serves_model';

/**
 * Extra detail-page tabs, per node kind.
 * Every node gets Graph and Properties.
 */
export interface KindDetailTab {
  value: string;
  config: RelatedNodesConfig;
  /** Land on this tab instead of Graph when opening the page. */
  primary?: boolean;
}

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
    relationKind: RELATION_KIND_SERVES_MODEL,
    direction: 'outgoing',
    title: 'Uses Model',
    icon: BrainCircuit,
    description: 'The model this inference endpoint serves.',
  },
  model: {
    relationKind: RELATION_KIND_SERVES_MODEL,
    direction: 'incoming',
    title: 'Served By',
    icon: Cloud,
    description: 'The inference endpoint that serves this model.',
  },
};

export const KIND_DETAIL_TABS: Record<string, KindDetailTab[]> = {
  [MCP_SERVER_KIND]: [{ value: 'Tools', config: RELATED_CARDS_BY_KIND["mcp_server"], primary: true }],
};

export function detailTabsForKind(kind: string): KindDetailTab[] {
  return KIND_DETAIL_TABS[kind] ?? [];
}

export function relatedCardForKind(kind: string): RelatedNodesConfig | undefined {
  return RELATED_CARDS_BY_KIND[kind];
}