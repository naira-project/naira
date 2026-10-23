import { Fragment } from 'react';
import { Link } from 'react-router';
import type { RadarCatalogNodes } from '@/hooks/useRadarCatalogNodes';
import { encodeCatalogPath, type NodeResource } from '@/lib/catalogApi';
import { type CatalogRef, matchCatalogRefs } from '@/lib/techRadar';

interface CatalogMatchesProps {
  refs: CatalogRef[];
  catalog: RadarCatalogNodes;
}

// Listed kinds come first, in this order; other kinds follow by name and are
// shown by their kind name.
const KIND_LABELS: Record<string, string> = {
  model: 'Models',
  inference_endpoint: 'Endpoints',
};
const KIND_ORDER = Object.keys(KIND_LABELS);

function kindRank(kind: string): number {
  const rank = KIND_ORDER.indexOf(kind);
  return rank === -1 ? KIND_ORDER.length : rank;
}

/**
 * Links a radar entry to the catalog nodes its refs match, grouped by kind:
 * e.g. the models where it is available and the endpoints that serve it.
 */
export default function CatalogMatches({ refs, catalog }: CatalogMatchesProps) {
  if (refs.length === 0) {
    return <span className="text-xs text-muted-foreground">—</span>;
  }
  if (catalog.loading) {
    return <span className="text-xs text-muted-foreground">Loading…</span>;
  }
  if (catalog.error) {
    return <span className="text-xs text-destructive">{catalog.error}</span>;
  }

  const matches = matchCatalogRefs(refs, catalog.nodes);
  if (matches.length === 0) {
    return <span className="text-xs text-muted-foreground">No matches</span>;
  }

  const byKind = new Map<string, NodeResource[]>();
  for (const node of matches) {
    byKind.set(node.kind, [...(byKind.get(node.kind) ?? []), node]);
  }
  const groups = Array.from(byKind).sort(
    ([a], [b]) => kindRank(a) - kindRank(b) || a.localeCompare(b),
  );
  return (
    <div className="flex flex-col gap-1">
      {groups.map(([kind, nodes]) => (
        <MatchGroup key={kind} label={KIND_LABELS[kind] ?? kind} nodes={nodes} />
      ))}
    </div>
  );
}

function MatchGroup({ label, nodes }: { label: string; nodes: NodeResource[] }) {
  return (
    <div className="text-sm">
      <span className="text-xs font-medium text-muted-foreground">{label}: </span>
      {nodes.map((node, index) => (
        <Fragment key={node.name}>
          {index > 0 && ', '}
          <Link
            to={`/catalog/${encodeURIComponent(node.kind)}/${encodeCatalogPath(node.path)}`}
            className="break-all text-primary hover:underline"
          >
            {node.path}
          </Link>
        </Fragment>
      ))}
    </div>
  );
}
