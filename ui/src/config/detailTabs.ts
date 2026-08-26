import type { ComponentType } from 'react';
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

export const KIND_DETAIL_TABS: Record<string, KindDetailTab[]> = {
  [MCP_SERVER_KIND]: [{ value: 'Tools', component: MCPServerTools, primary: true }],
};

export function detailTabsForKind(kind: string): KindDetailTab[] {
  return KIND_DETAIL_TABS[kind] ?? [];
}
