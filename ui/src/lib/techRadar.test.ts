import { describe, expect, it } from 'vitest';
import type { NodeResource } from './catalogApi';
import {
  type CatalogRef,
  catalogRefKinds,
  entriesMatchingNode,
  matchCatalogRefs,
  matchesCatalogRef,
  parseRadarModel,
  type RadarEntry,
} from './techRadar';

function radarNode(props: Record<string, string>): NodeResource {
  return {
    name: 'nodes/tech_radar/naira',
    kind: 'tech_radar',
    path: 'naira',
    pluginClaims: [{ plugin: 'tech-radar', props }],
  };
}

function entryNode(path: string, props: Record<string, string>): NodeResource {
  return {
    name: `nodes/tech_radar_entry/${path}`,
    kind: 'tech_radar_entry',
    path,
    pluginClaims: [{ plugin: 'tech-radar', props }],
  };
}

const validRadarProps = {
  title: 'Naira Tech Radar',
  edition: '2026-09',
  owner: 'platform-team',
  schema_version: '1',
  quadrants: JSON.stringify([
    { id: 'models', name: 'Models' },
    { id: 'agentic', name: 'Agentic Patterns' },
    { id: 'knowledge', name: 'Knowledge Techniques' },
    { id: 'others', name: 'Others' },
  ]),
  rings: JSON.stringify([
    { id: 'adopt', name: 'Adopt', description: 'Use it.' },
    { id: 'hold', name: 'Hold', description: 'Avoid it.' },
  ]),
  entry_count: '2',
  moved_count: '1',
};

describe('parseRadarModel', () => {
  it('parses a radar with entries sorted by index', () => {
    const model = parseRadarModel(radarNode(validRadarProps), [
      entryNode('naira/second', {
        title: 'Second',
        quadrant: 'knowledge',
        ring: 'hold',
        moved: 'none',
        owner: 'ai-board',
        rationale: 'superseded',
        index: '1',
      }),
      entryNode('naira/first', {
        title: 'First',
        quadrant: 'models',
        ring: 'adopt',
        moved: 'in',
        owner: 'ml-platform',
        rationale: 'default choice',
        index: '0',
      }),
    ]);

    expect(model).not.toBeNull();
    expect(model?.title).toBe('Naira Tech Radar');
    expect(model?.edition).toBe('2026-09');
    expect(model?.quadrants.map((q) => q.id)).toEqual(['models', 'agentic', 'knowledge', 'others']);
    expect(model?.rings[1].description).toBe('Avoid it.');
    expect(model?.entries.map((e) => e.name)).toEqual(['First', 'Second']);
    expect(model?.entries[0].moved).toBe('in');
    expect(model?.movedCount).toBe(1);
    expect(model?.orphans).toEqual([]);
  });

  it('returns null without a radar node', () => {
    expect(parseRadarModel(undefined, [])).toBeNull();
  });

  it('returns null for malformed taxonomy JSON', () => {
    expect(
      parseRadarModel(radarNode({ ...validRadarProps, quadrants: 'not-json' }), []),
    ).toBeNull();
    expect(
      parseRadarModel(radarNode({ ...validRadarProps, rings: '{"id":"adopt"}' }), []),
    ).toBeNull();
  });

  it('returns null when the taxonomy does not carry exactly 4 quadrants', () => {
    // The chart geometry hard-assumes 4 quadrants; anything else must degrade
    // to the not-synced state instead of crashing during render.
    const threeQuadrants = JSON.stringify([
      { id: 'a', name: 'A' },
      { id: 'b', name: 'B' },
      { id: 'c', name: 'C' },
    ]);
    expect(
      parseRadarModel(radarNode({ ...validRadarProps, quadrants: threeQuadrants }), []),
    ).toBeNull();
  });

  it('ignores entries belonging to a different radar', () => {
    const model = parseRadarModel(radarNode(validRadarProps), [
      entryNode('other/foreign', {
        title: 'Foreign',
        quadrant: 'models',
        ring: 'adopt',
        moved: 'none',
        owner: 'team',
        rationale: 'belongs to another radar',
        index: '0',
        radar: 'other',
      }),
      entryNode('naira/ours', {
        title: 'Ours',
        quadrant: 'models',
        ring: 'adopt',
        moved: 'none',
        owner: 'team',
        rationale: 'belongs here',
        index: '1',
        radar: 'naira',
      }),
    ]);

    expect(model?.entries.map((e) => e.name)).toEqual(['Ours']);
    expect(model?.orphans).toEqual([]);
  });

  it('collects entries with unknown taxonomy ids as orphans', () => {
    const model = parseRadarModel(radarNode(validRadarProps), [
      entryNode('naira/ghost', {
        title: 'Ghost',
        quadrant: 'models',
        ring: 'renamed-ring',
        moved: 'none',
        owner: 'team',
        rationale: 'stale',
        index: '0',
      }),
    ]);

    expect(model?.entries).toEqual([]);
    expect(model?.orphans.map((e) => e.name)).toEqual(['Ghost']);
  });

  it('parses catalog refs and skips malformed ones', () => {
    const entryProps = {
      quadrant: 'models',
      ring: 'adopt',
      moved: 'none',
      owner: 'team',
      rationale: 'r',
    };
    const model = parseRadarModel(radarNode(validRadarProps), [
      entryNode('naira/linked', {
        ...entryProps,
        title: 'Linked',
        index: '0',
        catalog_refs: JSON.stringify([
          { match: 'claude-sonnet', kinds: ['model', 'inference_endpoint'] },
          { kinds: ['model'], plugins: ['litellm'] },
          { match: 'no-kinds' },
          'old-style string',
        ]),
      }),
      entryNode('naira/unlinked', { ...entryProps, title: 'Unlinked', index: '1' }),
      entryNode('naira/broken', {
        ...entryProps,
        title: 'Broken',
        index: '2',
        catalog_refs: '{"not":"a list"}',
      }),
    ]);

    expect(model?.entries.map((e) => e.catalogRefs)).toEqual([
      [
        { match: 'claude-sonnet', kinds: ['model', 'inference_endpoint'], plugins: [] },
        { match: '', kinds: ['model'], plugins: ['litellm'] },
      ],
      [],
      [],
    ]);
  });
});

function catalogNode(kind: string, path: string, plugins: string[] = []): NodeResource {
  return {
    name: `nodes/${kind}/${path}`,
    kind,
    path,
    pluginClaims: plugins.map((plugin) => ({ plugin, props: {} })),
  };
}

function ref(overrides: Partial<CatalogRef>): CatalogRef {
  return { match: '', kinds: ['model', 'inference_endpoint'], plugins: [], ...overrides };
}

describe('matchesCatalogRef', () => {
  const sonnet = catalogNode('model', 'litellm/Claude-Sonnet-4', ['litellm']);

  it('matches the path ignoring case, within the listed kinds', () => {
    expect(matchesCatalogRef(sonnet, ref({ match: 'claude-sonnet' }))).toBe(true);
    expect(matchesCatalogRef(sonnet, ref({ match: 'claude-opus' }))).toBe(false);
    expect(
      matchesCatalogRef(sonnet, ref({ match: 'claude-sonnet', kinds: ['inference_endpoint'] })),
    ).toBe(false);
  });

  it('requires one of the plugins to have emitted the node', () => {
    expect(matchesCatalogRef(sonnet, ref({ plugins: ['litellm'] }))).toBe(true);
    expect(matchesCatalogRef(sonnet, ref({ plugins: ['bedrock', 'litellm'] }))).toBe(true);
    expect(matchesCatalogRef(sonnet, ref({ plugins: ['bedrock'] }))).toBe(false);
    expect(matchesCatalogRef(sonnet, ref({ match: 'opus', plugins: ['litellm'] }))).toBe(false);
  });

  it('matches nothing without match and plugins', () => {
    expect(matchesCatalogRef(sonnet, ref({}))).toBe(false);
  });
});

describe('matchCatalogRefs', () => {
  const nodes = [
    catalogNode('model', 'litellm/claude-sonnet-4'),
    catalogNode('inference_endpoint', 'litellm/claude-sonnet-4/abc'),
    catalogNode('model', 'bedrock/anthropic.claude-sonnet-4-20250514-v1:0'),
    catalogNode('model', 'mlflow/fraud-detector'),
    catalogNode('model', 'bedrock/anthropic.Claude-Opus-4-1-v1:0'),
  ];

  it('returns each matching node once, sorted by kind and then path', () => {
    const refs = [ref({ match: 'claude-sonnet' }), ref({ match: 'claude' })];
    expect(matchCatalogRefs(refs, nodes).map((n) => `${n.kind}:${n.path}`)).toEqual([
      'inference_endpoint:litellm/claude-sonnet-4/abc',
      'model:bedrock/anthropic.Claude-Opus-4-1-v1:0',
      'model:bedrock/anthropic.claude-sonnet-4-20250514-v1:0',
      'model:litellm/claude-sonnet-4',
    ]);
  });

  it('matches nothing without refs', () => {
    expect(matchCatalogRefs([], nodes)).toEqual([]);
  });
});

function radarEntry(path: string, catalogRefs: CatalogRef[]): RadarEntry {
  return {
    path,
    name: path,
    quadrant: 'models',
    ring: 'adopt',
    moved: 'none',
    owner: 'team',
    rationale: 'r',
    index: 0,
    catalogRefs,
  };
}

describe('catalogRefKinds', () => {
  it('collects the kinds of every ref, deduplicated and sorted', () => {
    const entries = [
      radarEntry('naira/a', [ref({ match: 'a' })]),
      radarEntry('naira/b', [ref({ match: 'b', kinds: ['deployment', 'model'] })]),
      radarEntry('naira/c', []),
    ];
    expect(catalogRefKinds(entries)).toEqual(['deployment', 'inference_endpoint', 'model']);
  });
});

describe('entriesMatchingNode', () => {
  it('returns the entries whose refs match the node, the reverse of matchCatalogRefs', () => {
    const entries = [
      radarEntry('naira/sonnet', [ref({ match: 'claude-sonnet' })]),
      radarEntry('naira/litellm', [ref({ plugins: ['litellm'] })]),
      radarEntry('naira/opus', [ref({ match: 'claude-opus' })]),
    ];
    const node = catalogNode('model', 'litellm/claude-sonnet-4', ['litellm']);
    expect(entriesMatchingNode(entries, node).map((e) => e.path)).toEqual([
      'naira/sonnet',
      'naira/litellm',
    ]);
  });
});
