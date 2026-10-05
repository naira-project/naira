import type { ComponentType } from 'react';
import LangfuseMetrics from '../components/LangfuseMetrics';
import MCPServerTools, { MCP_SERVER_KIND } from '../components/MCPServerTools';
import type { NodeResource } from '../lib/catalogApi';

/**
 * Extra detail-page tabs, per node kind.
 * Every node gets Graph and Properties.
 */
export interface KindDetailTab {
  value: string;
  component: ComponentType<{ node: NodeResource }>;
  /** Land on this tab instead of Graph when opening the page. */
  primary?: boolean;
}

/**
 * Kinds that can carry LLM observability. The tab renders its own empty
 * state, so listing a kind costs nothing for nodes with no Langfuse project.
 */
const LANGFUSE_OBSERVABLE_KINDS = ['deployment', 'application'];

export const KIND_DETAIL_TABS: Record<string, KindDetailTab[]> = {
  [MCP_SERVER_KIND]: [{ value: 'Tools', component: MCPServerTools, primary: true }],
  ...Object.fromEntries(
    LANGFUSE_OBSERVABLE_KINDS.map((kind) => [
      kind,
      // Not primary: the graph stays the point of a deployment's page.
      [{ value: 'Langfuse', component: LangfuseMetrics }],
    ]),
  ),
};

export function detailTabsForKind(kind: string): KindDetailTab[] {
  return KIND_DETAIL_TABS[kind] ?? [];
}
