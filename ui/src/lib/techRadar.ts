import { type NodeResource, nodeProps } from './catalogApi';

export const TECH_RADAR_KIND = 'tech_radar';
export const TECH_RADAR_ENTRY_KIND = 'tech_radar_entry';
export const TECH_RADAR_PLUGIN = 'tech-radar';

export interface RadarQuadrant {
  id: string;
  name: string;
}

export interface RadarRing {
  id: string;
  name: string;
  description: string;
}

export type RadarMovement = 'in' | 'out' | 'none';

export interface RadarEntry {
  /** Node path, e.g. "naira/claude-sonnet". */
  path: string;
  name: string;
  quadrant: string;
  ring: string;
  moved: RadarMovement;
  owner: string;
  rationale: string;
  /** Position in the config file; drives the blip numbering. */
  index: number;
}

export interface RadarModel {
  title: string;
  edition: string;
  owner: string;
  quadrants: RadarQuadrant[];
  rings: RadarRing[];
  /** Entries with a resolvable quadrant and ring, sorted by config order. */
  entries: RadarEntry[];
  /**
   * Entries referencing a quadrant or ring id missing from the taxonomy.
   * Can occur transiently when a taxonomy rename is cached mid-sync.
   */
  orphans: RadarEntry[];
  movedCount: number;
}

/**
 * Colors assigned to rings by taxonomy index (innermost first). The classic
 * adopt/trial/assess/hold semantics map to green → teal → amber → red.
 */
const RING_COLORS = ['#16a34a', '#0097a7', '#f59e0b', '#dc2626', '#7c3aed', '#6b7280'];

export function ringColor(ringIndex: number): string {
  return RING_COLORS[ringIndex] ?? '#6b7280';
}

function parseMovement(value: string | undefined): RadarMovement {
  return value === 'in' || value === 'out' ? value : 'none';
}

function parseTaxonomy<T>(raw: string | undefined, isValid: (item: T) => boolean): T[] | null {
  if (!raw) {
    return null;
  }
  try {
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed) || !parsed.every(isValid)) {
      return null;
    }
    return parsed as T[];
  } catch {
    return null;
  }
}

/**
 * Assemble the radar view model from the tech_radar node and its entry nodes.
 * Returns null when the radar node is absent or carries a broken taxonomy.
 */
export function parseRadarModel(
  radarNode: NodeResource | undefined,
  entryNodes: NodeResource[],
): RadarModel | null {
  if (!radarNode) {
    return null;
  }

  const props = nodeProps(radarNode);
  const quadrants = parseTaxonomy<RadarQuadrant>(
    props.quadrants,
    (q) => typeof q?.id === 'string' && typeof q?.name === 'string',
  );
  const rings = parseTaxonomy<RadarRing>(
    props.rings,
    (r) => typeof r?.id === 'string' && typeof r?.name === 'string',
  );
  if (!quadrants || !rings) {
    return null;
  }

  const quadrantIds = new Set(quadrants.map((q) => q.id));
  const ringIds = new Set(rings.map((r) => r.id));

  const allEntries = entryNodes
    .map((node): RadarEntry => {
      const entryProps = nodeProps(node);
      return {
        path: node.path,
        name: entryProps.title ?? node.path,
        quadrant: entryProps.quadrant ?? '',
        ring: entryProps.ring ?? '',
        moved: parseMovement(entryProps.moved),
        owner: entryProps.owner ?? '',
        rationale: entryProps.rationale ?? '',
        index: Number.parseInt(entryProps.index ?? '0', 10) || 0,
      };
    })
    .sort((a, b) => a.index - b.index);

  const entries = allEntries.filter((e) => quadrantIds.has(e.quadrant) && ringIds.has(e.ring));
  const orphans = allEntries.filter((e) => !quadrantIds.has(e.quadrant) || !ringIds.has(e.ring));

  return {
    title: props.title ?? 'Tech Radar',
    edition: props.edition ?? '',
    owner: props.owner ?? '',
    quadrants,
    rings,
    entries,
    orphans,
    movedCount: entries.filter((e) => e.moved !== 'none').length,
  };
}
