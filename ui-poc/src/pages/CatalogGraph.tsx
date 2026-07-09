import React, { useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router';
import {
  ArrowRight,
  GitBranch,
  GripVertical,
  Network,
  RefreshCw,
  Share2,
} from 'lucide-react';
import {
  Background,
  Controls,
  MarkerType,
  MiniMap,
  ReactFlow,
  useEdgesState,
  useNodesState,
  type Edge,
  type Node,
  type ReactFlowInstance,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';

import { Badge } from '../components/ui/badge';
import { Card, CardContent } from '../components/ui/card';
import { Input } from '../components/ui/input';
import { Separator } from '../components/ui/separator';
import { useCatalogGraph, type CatalogGraphEdge, type CatalogGraphNode, type CatalogGraphRoot } from '../hooks/useCatalogGraph';

// TODO: how to make in a way that is not hardcoded?
const typePalette: Record<string, { fill: string; stroke: string }> = {
  application: { fill: '#dff4ff', stroke: '#0b6fa4' },
  model: { fill: '#fff1d6', stroke: '#b36b00' },
  dataset: { fill: '#def7e5', stroke: '#13795b' },
  deployment: { fill: '#f3e8ff', stroke: '#6b21a8' },
  service: { fill: '#fce7f3', stroke: '#9d174d' },
  'Kustomization.fluxcd': { fill: '#e0f2fe', stroke: '#0369a1' },
  'HelmChart.fluxcd': { fill: '#fef3c7', stroke: '#b45309' },
  git_repository: { fill: '#e0e7ff', stroke: '#3730a3' },
};

function graphNodeId(node: CatalogGraphNode) {
  return node.name;
}

function toFlowNode(node: CatalogGraphNode, position: { x: number; y: number }): Node {
  const palette = typePalette[node.kind] ?? { fill: '#ffffff', stroke: '#94a3b8' };

  const displayLabel = (
    <div className="flex flex-col gap-1.5 text-left">
      <div className="flex items-center justify-between gap-1.5">
        <span className="text-[10px] font-bold uppercase tracking-wider" style={{ color: palette.stroke }}>
          {node.kind}
        </span>
        <GripVertical
          size={12}
          className="catalog-node-drag-handle cursor-grab text-gray-300 hover:text-gray-500"
        />
      </div>
      <span className="font-semibold break-all text-sm leading-tight text-[#17324d]">
        {node.label}
      </span>
    </div>
  );

  return {
    id: graphNodeId(node),
    position,
    dragHandle: '.catalog-node-drag-handle',
    data: {
      label: displayLabel,
      path: node.path,
      nodeKind: node.kind,
      depth: node.depth,
      isRoot: node.isRoot,
    },
    draggable: true,
    selectable: true,
    style: {
      cursor: 'pointer',
      width: 220,
      borderRadius: 10,
      border: '1px solid #e2e8f0',
      borderLeft: `6px solid ${palette.stroke}`,
      background: node.isRoot ? '#f0faf4' : '#ffffff',
      boxShadow: node.isRoot
        ? '0 20px 40px rgba(15, 23, 42, 0.12)'
        : '0 12px 28px rgba(15, 23, 42, 0.06)',
      padding: '14px 14px 14px 12px',
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

type CatalogGraphProps = {
  rootNode: CatalogGraphRoot;
};

export default function CatalogGraph({ rootNode }: CatalogGraphProps) {
  const navigate = useNavigate();
  const [depth, setDepth] = useState(1);
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const { graph, loading, error, reload } = useCatalogGraph(rootNode, depth);

  // The state of nodes/edges is controlled by React Flow, allowing the
  // user to drag them. The layout is recalculated from scratch on every
  // data change from the hook (e.g., after a depth change or reload).
  const [flowNodes, setFlowNodes, onNodesChange] = useNodesState<Node>([]);
  const [flowEdges, setFlowEdges, onEdgesChange] = useEdgesState<Edge>([]);
  const instanceRef = useRef<ReactFlowInstance | null>(null);

  useEffect(() => {
    setFlowNodes(layoutNodes(graph.nodes));
    setFlowEdges(graph.edges.map(toFlowEdge));
  }, [graph, setFlowNodes, setFlowEdges]);

  // Auto-fit view after each layout change (e.g., depth change or reload).
  useEffect(() => {
    requestAnimationFrame(() => {
      instanceRef.current?.fitView();
    });
  }, [graph]);

  const selectedNode = graph.nodes.find((node) => graphNodeId(node) === selectedNodeId) ?? graph.nodes[0] ?? null;
  const incomingCount = graph.edges.filter((edge) => edge.direction === 'incoming').length;
  const outgoingCount = graph.edges.filter((edge) => edge.direction === 'outgoing').length;

  const handleNodeClick = (_: React.MouseEvent, node: Node) => {
    setSelectedNodeId(node.id);

    const graphNode = graph.nodes.find((item) => graphNodeId(item) === node.id);
    if (!graphNode) {
      return;
    }

    navigate(`/catalog/${graphNode.kind}/${encodeURIComponent(graphNode.path)}`);
  };

  return (
    <div className="flex flex-col gap-3">
      {/* Compact toolbar: depth controls + inline stats */}
      <div className="flex flex-wrap items-center gap-3">
        <div className="flex items-center gap-2">
          <label className="text-[0.65rem] font-semibold uppercase tracking-[0.18em] text-foreground-secondary">
            Depth
          </label>
          <Input
            type="number"
            min={1}
            value={depth}
            onChange={(event) => setDepth(Math.max(1, Number(event.target.value) || 1))}
            className="w-16 h-8"
          />
          <button
            onClick={reload}
            className="inline-flex items-center gap-1 rounded-lg bg-primary px-3 py-1.5 text-xs font-semibold text-white transition-colors hover:bg-primary-dark"
          >
            <RefreshCw size={14} />
            Reload
          </button>
        </div>

        <div className="flex-1 min-w-4" />

        <div className="flex items-center gap-3 text-xs text-foreground-secondary">
          <span className="inline-flex items-center gap-1">
            <Network size={14} className="text-primary" />
            <strong className="text-foreground">{graph.nodes.length}</strong> nodes
          </span>
          <span className="inline-flex items-center gap-1">
            <Share2 size={14} className="text-primary" />
            <strong className="text-foreground">{graph.edges.length}</strong> edges
          </span>
          <span className="inline-flex items-center gap-1">
            <ArrowRight size={14} className="text-[#7b4bb3]" />
            <strong className="text-foreground">{incomingCount}</strong> in
          </span>
          <span className="inline-flex items-center gap-1">
            <ArrowRight size={14} className="text-[#3b6a8a]" />
            <strong className="text-foreground">{outgoingCount}</strong> out
          </span>
          <span className="inline-flex items-center gap-1">
            <GitBranch size={14} className="text-primary" />
            <strong className="text-foreground">{depth}</strong> depth
          </span>
        </div>
      </div>

      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_320px]">
        <Card className="min-h-0 overflow-hidden rounded-[20px] border-gray-200 dark:border-gray-700">
          <div className="relative h-[580px]">
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
              onNodesChange={onNodesChange}
              onEdgesChange={onEdgesChange}
              onInit={(instance) => {
                instanceRef.current = instance;
              }}
              fitView
              onNodeClick={handleNodeClick}
              nodesDraggable
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
                  const palette = typePalette[typedNode?.kind ?? ''] ?? { fill: '#ced6de', stroke: '#94a3b8' };
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