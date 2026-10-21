import { type NodeResource, nodeProps } from './catalogApi';
import { nodePlugins } from './kindUtils';

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
  /** Links to the catalog nodes the entry is used as; see matchesCatalogRef. */
  catalogRefs: CatalogRef[];
}

/** One catalog_refs item of a radar entry, as emitted by the tech-radar plugin. */
export interface CatalogRef {
  /** Substring of the node path, matched ignoring case; empty matches any path. */
  match: string;
  /** Node kinds to match; the plugin fills in the default. */
  kinds: string[];
  /** Plugins of which one must have emitted the node; empty matches any plugin. */
  plugins: string[];
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

function isStringArray(value: unknown): value is string[] {
  return Array.isArray(value) && value.every((item) => typeof item === 'string');
}

/** Parses the catalog_refs property, skipping items that are not well-formed refs. */
function parseCatalogRefs(raw: string | undefined): CatalogRef[] {
  if (!raw) {
    return [];
  }
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return [];
  }
  if (!Array.isArray(parsed)) {
    return [];
  }
  return parsed.flatMap((item): CatalogRef[] => {
    if (typeof item !== 'object' || item === null) {
      return [];
    }
    const { match = '', kinds, plugins = [] } = item as Record<string, unknown>;
    if (typeof match !== 'string' || !isStringArray(kinds) || !isStringArray(plugins)) {
      return [];
    }
    return [{ match, kinds, plugins }];
  });
}

/**
 * Reports whether `node` is one the ref points at: its kind is listed, its
 * path contains the ref's match ignoring case, and one of the ref's plugins
 * emitted it. A ref without match and plugins matches nothing, so a
 * malformed ref cannot link an entry to every node of a kind.
 */
export function matchesCatalogRef(node: NodeResource, ref: CatalogRef): boolean {
  if (ref.match === '' && ref.plugins.length === 0) {
    return false;
  }
  if (!ref.kinds.includes(node.kind)) {
    return false;
  }
  if (ref.match !== '' && !node.path.toLowerCase().includes(ref.match.toLowerCase())) {
    return false;
  }
  return (
    ref.plugins.length === 0 || nodePlugins(node).some((plugin) => ref.plugins.includes(plugin))
  );
}

/**
 * Returns the nodes matching any of the refs, sorted by kind and then path.
 * One ref can match the same model under several providers.
 */
export function matchCatalogRefs(refs: CatalogRef[], nodes: NodeResource[]): NodeResource[] {
  return nodes
    .filter((node) => refs.some((ref) => matchesCatalogRef(node, ref)))
    .sort((a, b) => a.kind.localeCompare(b.kind) || a.path.localeCompare(b.path));
}

/** The node kinds that the entries' catalog refs can match, sorted. */
export function catalogRefKinds(entries: RadarEntry[]): string[] {
  const kinds = new Set(entries.flatMap((entry) => entry.catalogRefs.flatMap((ref) => ref.kinds)));
  return Array.from(kinds).sort();
}

/** The entries with a catalog ref matching `node`, in radar order. */
export function entriesMatchingNode(entries: RadarEntry[], node: NodeResource): RadarEntry[] {
  return entries.filter((entry) => entry.catalogRefs.some((ref) => matchesCatalogRef(node, ref)));
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
  // The chart geometry (quadrantStartAngle, QUADRANT_LABELS in RadarChart)
  // hard-assumes exactly 4 quadrants, matching the plugin's validation.
  // Reject anything else here so a version-skewed or foreign claim degrades
  // to the not-synced state instead of crashing the page during render.
  if (quadrants?.length !== 4 || !rings) {
    return null;
  }

  const quadrantIds = new Set(quadrants.map((q) => q.id));
  const ringIds = new Set(rings.map((r) => r.id));

  const allEntries = entryNodes
    .map((node) => ({ node, entryProps: nodeProps(node) }))
    // Entries are namespaced by radar id (the plugin writes a "radar" prop and
    // prefixes paths with it). Keep only this radar's entries so a second
    // radar's blips never leak into the view; entries without the prop are
    // kept for backward compatibility.
    .filter(({ entryProps }) => (entryProps.radar ?? radarNode.path) === radarNode.path)
    .map(({ node, entryProps }): RadarEntry => {
      return {
        path: node.path,
        name: entryProps.title ?? node.path,
        quadrant: entryProps.quadrant ?? '',
        ring: entryProps.ring ?? '',
        moved: parseMovement(entryProps.moved),
        owner: entryProps.owner ?? '',
        rationale: entryProps.rationale ?? '',
        index: Number.parseInt(entryProps.index ?? '0', 10) || 0,
        catalogRefs: parseCatalogRefs(entryProps.catalog_refs),
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
