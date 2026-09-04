import { describe, expect, it } from 'vitest';
import type { NodeResource } from './catalogApi';
import { parseRadarModel } from './techRadar';

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
});
