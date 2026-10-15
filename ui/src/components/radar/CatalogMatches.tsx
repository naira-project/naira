import { Fragment } from 'react';
import { Link } from 'react-router';
import type { RadarCatalogNodes } from '@/hooks/useRadarCatalogNodes';
import { encodeCatalogPath, type NodeResource } from '@/lib/catalogApi';
import { matchCatalogRefs } from '@/lib/techRadar';

interface CatalogMatchesProps {
  refs: string[];
  catalog: RadarCatalogNodes;
}

/**
 * Links a radar entry to the catalog nodes its refs match: the models (where
 * it is available) and the inference endpoints (where it is served).
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

  return (
    <div className="flex flex-col gap-1">
      <MatchGroup label="Models" nodes={matchCatalogRefs(refs, catalog.models)} />
      <MatchGroup label="Endpoints" nodes={matchCatalogRefs(refs, catalog.endpoints)} />
    </div>
  );
}

function MatchGroup({ label, nodes }: { label: string; nodes: NodeResource[] }) {
  return (
    <div className="text-sm">
      <span className="text-xs font-medium text-muted-foreground">{label}: </span>
      {nodes.length === 0 ? (
        <span className="text-xs text-muted-foreground">none</span>
      ) : (
        nodes.map((node, index) => (
          <Fragment key={node.name}>
            {index > 0 && ', '}
            <Link
              to={`/catalog/${encodeURIComponent(node.kind)}/${encodeCatalogPath(node.path)}`}
              className="break-all text-primary hover:underline"
            >
              {node.path}
            </Link>
          </Fragment>
        ))
      )}
    </div>
  );
}
