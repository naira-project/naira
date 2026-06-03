import React, { useMemo, useState } from 'react';
import { ArrowRight, GitBranch, Network, RefreshCw, Share2 } from 'lucide-react';
import {
  Background,
  Controls,
  MarkerType,
  MiniMap,
  ReactFlow,
  type Edge,
  type Node,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';

import { Badge } from '../components/ui/badge';
import { Card, CardContent } from '../components/ui/card';
import { Input } from '../components/ui/input';
import { Separator } from '../components/ui/separator';
import { useCatalogGraph, type CatalogGraphEdge, type CatalogGraphNode, type CatalogGraphRoot } from '../hooks/useCatalogGraph';

const typePalette: Record<string, { fill: string; stroke: string }> = {
  application: { fill: '#dff4ff', stroke: '#0b6fa4' },
  model: { fill: '#fff1d6', stroke: '#b36b00' },
  dataset: { fill: '#def7e5', stroke: '#13795b' },
};

function graphNodeId(node: CatalogGraphNode) {
  return node.name;
}

function toFlowNode(node: CatalogGraphNode, position: { x: number; y: number }): Node {
  const palette = typePalette[node.kind] ?? { fill: '#eceff3', stroke: '#516170' };

  return {
    id: graphNodeId(node),
    position,
    data: {
      label: node.label,
      path: node.path,
      nodeKind: node.kind,
      depth: node.depth,
      isRoot: node.isRoot,
    },
    draggable: false,
    selectable: true,
    style: {
      width: 220,
      borderRadius: 18,
      border: `2px solid ${palette.stroke}`,
      background: node.isRoot ? '#eef6ff' : palette.fill,
      boxShadow: node.isRoot ? '0 20px 40px rgba(15, 23, 42, 0.12)' : '0 12px 28px rgba(15, 23, 42, 0.06)',
      padding: 16,
      color: '#17324d',
    },
  };
}

function layoutNodes(nodes: CatalogGraphNode[]): Node[] {
  const grouped = new Map<number, CatalogGraphNode[]>();
  const sortedDepths = Array.from(new Set(nodes.map((node) => node.depth))).sort((left, right) => left - right);
  const depthOffsets = new Map(sortedDepths.map((depth, index) => [depth, index]));

  for (const node of nodes) {
    const current = grouped.get(node.depth) ?? [];
    current.push(node);
    grouped.set(node.depth, current);
  }

  const positioned: Node[] = [];

  Array.from(grouped.entries())
    .sort(([leftDepth], [rightDepth]) => leftDepth - rightDepth)
    .forEach(([depth, groupNodes]) => {
      const orderedNodes = [...groupNodes].sort((left, right) => {
        if (left.kind === right.kind) {
          return left.path.localeCompare(right.path);
        }
        return left.kind.localeCompare(right.kind);
      });

      orderedNodes.forEach((node, index) => {
        positioned.push(toFlowNode(node, {
          x: (depthOffsets.get(depth) ?? 0) * 300,
          y: index * 150,
        }));
      });
    });

  return positioned;
}

function toFlowEdge(edge: CatalogGraphEdge): Edge {
  return {
    id: edge.id,
    source: edge.fromNode,
    target: edge.toNode,
    label: edge.kind,
    animated: false,
    markerEnd: {
      type: MarkerType.ArrowClosed,
      width: 18,
      height: 18,
      color: edge.direction === 'incoming' ? '#7b4bb3' : '#3b6a8a',
    },
    style: {
      stroke: edge.direction === 'incoming' ? '#7b4bb3' : '#3b6a8a',
      strokeWidth: 2,
    },
    labelStyle: {
      fill: edge.direction === 'incoming' ? '#5e3789' : '#24455f',
      fontWeight: 700,
      fontSize: 11,
    },
    labelBgStyle: {
      fill: '#ffffff',
      fillOpacity: 0.88,
    },
  };
}

function StatCard({ label, value, icon }: { label: string; value: number; icon: React.ReactNode }) {
  return (
    <Card className="min-w-36 border-gray-200 dark:border-gray-700">
      <CardContent className="flex items-start gap-3 py-3.5">
        <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-primary/10 text-primary">
          {icon}
        </div>
        <div>
          <p className="text-[0.65rem] font-semibold uppercase tracking-[0.18em] text-foreground-secondary dark:text-foreground-dark-secondary">
            {label}
          </p>
          <p className="mt-1 text-3xl font-bold text-foreground dark:text-foreground-dark-default">
            {value}
          </p>
        </div>
      </CardContent>
    </Card>
  );
}

type CatalogGraphProps = {
  rootNode: CatalogGraphRoot;
};

export default function CatalogGraph({ rootNode }: CatalogGraphProps) {
  const [depth, setDepth] = useState(1);
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const { graph, loading, error, reload } = useCatalogGraph(rootNode, depth);

  const flowNodes = useMemo(() => layoutNodes(graph.nodes), [graph.nodes]);
  const flowEdges = useMemo(() => graph.edges.map(toFlowEdge), [graph.edges]);
  const selectedNode = graph.nodes.find((node) => graphNodeId(node) === selectedNodeId) ?? graph.nodes[0] ?? null;
  const incomingCount = graph.edges.filter((edge) => edge.direction === 'incoming').length;
  const outgoingCount = graph.edges.filter((edge) => edge.direction === 'outgoing').length;

  return (
    <div className="space-y-4">
      <Card className="border-gray-200 dark:border-gray-700">
        <CardContent className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <p className="text-[0.65rem] font-semibold uppercase tracking-[0.18em] text-foreground-secondary dark:text-foreground-dark-secondary">
              Root Node
            </p>
            <h2 className="mt-1 text-xl font-semibold text-foreground dark:text-foreground-dark-default">
              {rootNode.label}
            </h2>
            <p className="mt-1 font-mono text-sm text-foreground-secondary dark:text-foreground-dark-secondary">
              {rootNode.name}
            </p>
          </div>

          <div className="flex flex-wrap items-end gap-3">
            <div>
              <label className="mb-1 block text-[0.65rem] font-semibold uppercase tracking-[0.18em] text-foreground-secondary dark:text-foreground-dark-secondary">
                Depth
              </label>
              <Input
                type="number"
                min={1}
                value={depth}
                onChange={(event) => setDepth(Math.max(1, Number(event.target.value) || 1))}
                className="w-24"
              />
            </div>

            <button
              onClick={reload}
              className="inline-flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-primary-dark"
            >
              <RefreshCw size={16} />
              Reload
            </button>
          </div>
        </CardContent>
      </Card>

      <div className="flex flex-wrap gap-4">
        <StatCard label="Nodes" value={graph.nodes.length} icon={<Network size={18} />} />
        <StatCard label="Edges" value={graph.edges.length} icon={<Share2 size={18} />} />
        <StatCard label="Incoming" value={incomingCount} icon={<ArrowRight size={18} />} />
        <StatCard label="Outgoing" value={outgoingCount} icon={<ArrowRight size={18} />} />
        <StatCard label="Depth" value={depth} icon={<GitBranch size={18} />} />
      </div>

      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_320px]">
        <Card className="min-h-0 overflow-hidden rounded-[20px] border-gray-200 dark:border-gray-700">
          <div className="relative h-[520px]">
            {loading && (
              <div className="absolute inset-0 z-20 flex items-center justify-center bg-white/85 dark:bg-background-dark-paper/85">
                <p className="text-sm text-foreground-secondary dark:text-foreground-dark-secondary">Loading graph...</p>
              </div>
            )}

            {error && !loading && (
              <div className="absolute inset-0 z-20 flex items-center justify-center bg-white/90 dark:bg-background-dark-paper/90">
                <p className="text-sm font-medium text-red-500">{error}</p>
              </div>
            )}

            <ReactFlow
              nodes={flowNodes}
              edges={flowEdges}
              fitView
              onNodeClick={(_, node) => setSelectedNodeId(node.id)}
              nodesDraggable={false}
              nodesConnectable={false}
              elementsSelectable
              proOptions={{ hideAttribution: true }}
              defaultEdgeOptions={{ markerEnd: { type: MarkerType.ArrowClosed } }}
            >
              <MiniMap
                pannable
                zoomable
                nodeColor={(node) => {
                  const typedNode = graph.nodes.find((item) => graphNodeId(item) === node.id);
                  const palette = typePalette[typedNode?.kind ?? ''] ?? { fill: '#ced6de', stroke: '#516170' };
                  return typedNode?.isRoot ? '#0f5c61' : palette.stroke;
                }}
              />
              <Controls showInteractive={false} />
              <Background color="#d7e2ec" gap={20} size={1.2} />
            </ReactFlow>
          </div>
        </Card>

        <div className="space-y-4">
          <Card className="rounded-[20px] border-gray-200 dark:border-gray-700">
            <CardContent>
              <h2 className="text-base font-semibold text-foreground dark:text-foreground-dark-default">
                Graph Scope
              </h2>
              <p className="mt-2 text-sm text-foreground-secondary dark:text-foreground-dark-secondary">
                This view combines incoming and outgoing relations around the current root node.
              </p>
              <div className="mt-4 flex items-center gap-2 rounded-xl bg-gray-50 px-3 py-3 dark:bg-white/5">
                <span className="text-sm text-foreground-secondary dark:text-foreground-dark-secondary">Incoming</span>
                <ArrowRight size={16} className="rotate-180 text-[#7b4bb3]" />
                <Badge variant="default">Root</Badge>
                <span className="truncate text-sm text-foreground dark:text-foreground-dark-default">{rootNode.label}</span>
                <ArrowRight size={16} className="text-[#3b6a8a]" />
                <span className="text-sm text-foreground-secondary dark:text-foreground-dark-secondary">Outgoing</span>
              </div>
            </CardContent>
          </Card>

          <Card className="rounded-[20px] border-gray-200 dark:border-gray-700">
            <CardContent>
              <h2 className="text-base font-semibold text-foreground dark:text-foreground-dark-default">
                Node Details
              </h2>
              <p className="mt-1 text-sm text-foreground-secondary dark:text-foreground-dark-secondary">
                Selected node in the current rooted graph slice.
              </p>
              <Separator className="my-4" />

              {!selectedNode && (
                <p className="text-sm text-foreground-secondary dark:text-foreground-dark-secondary">
                  No node selected.
                </p>
              )}

              {selectedNode && (
                <div className="space-y-4">
                  <div>
                    <h3 className="text-xl font-bold text-foreground dark:text-foreground-dark-default">
                      {selectedNode.label}
                    </h3>
                  </div>

                  <div className="flex gap-2">
                    <Badge variant={selectedNode.isRoot ? 'default' : 'outline'} className="w-fit">
                      {selectedNode.isRoot ? 'Root node' : 'Discovered node'}
                    </Badge>
                    <Badge variant="outline" className="w-fit">
                      Depth {selectedNode.depth}
                    </Badge>
                  </div>

                  <div>
                    <p className="text-[0.65rem] font-semibold uppercase tracking-[0.18em] text-foreground-secondary dark:text-foreground-dark-secondary">
                      Kind
                    </p>
                    <p className="mt-1 text-sm text-foreground dark:text-foreground-dark-default">
                      {selectedNode.kind}
                    </p>
                  </div>

                  <div>
                    <p className="text-[0.65rem] font-semibold uppercase tracking-[0.18em] text-foreground-secondary dark:text-foreground-dark-secondary">
                      Path
                    </p>
                    <p className="mt-1 break-words font-mono text-sm text-foreground dark:text-foreground-dark-default">
                      {selectedNode.path}
                    </p>
                  </div>
                </div>
              )}
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}