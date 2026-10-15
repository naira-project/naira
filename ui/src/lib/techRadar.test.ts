import { describe, expect, it } from 'vitest';
import type { NodeResource } from './catalogApi';
import { matchCatalogRefs, parseRadarModel } from './techRadar';

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

  it('parses catalog refs and falls back to none when absent or malformed', () => {
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
        catalog_refs: '["claude-sonnet","Claude-Opus"]',
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
      ['claude-sonnet', 'Claude-Opus'],
      [],
      [],
    ]);
  });
});

describe('matchCatalogRefs', () => {
  function node(kind: string, path: string): NodeResource {
    return { name: `nodes/${kind}/${path}`, kind, path, pluginClaims: [] };
  }

  const nodes = [
    node('model', 'litellm/claude-sonnet-4'),
    node('model', 'bedrock/anthropic.claude-sonnet-4-20250514-v1:0'),
    node('model', 'mlflow/fraud-detector'),
    node('model', 'bedrock/anthropic.Claude-Opus-4-1-v1:0'),
  ];

  it('matches every provider path containing a ref, ignoring case, sorted by path', () => {
    expect(matchCatalogRefs(['claude-sonnet', 'claude-opus'], nodes).map((n) => n.path)).toEqual([
      'bedrock/anthropic.Claude-Opus-4-1-v1:0',
      'bedrock/anthropic.claude-sonnet-4-20250514-v1:0',
      'litellm/claude-sonnet-4',
    ]);
  });

  it('matches nothing without refs', () => {
    expect(matchCatalogRefs([], nodes)).toEqual([]);
  });
});
