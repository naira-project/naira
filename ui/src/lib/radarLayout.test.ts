import { describe, expect, it } from 'vitest';
import { BLIP_RADIUS, layoutBlips, quadrantStartAngle } from './radarLayout';
import type { RadarEntry } from './techRadar';

const QUADRANTS = ['models', 'agentic', 'knowledge', 'others'];
const RINGS = ['adopt', 'trial', 'assess', 'hold'];
const RADIUS = 350;

function makeEntry(overrides: Partial<RadarEntry> & { path: string; index: number }): RadarEntry {
  return {
    name: overrides.path,
    quadrant: 'models',
    ring: 'adopt',
    moved: 'none',
    owner: 'team',
    rationale: 'because',
    ...overrides,
  };
}

function makeEntries(count: number, quadrant: string, ring: string): RadarEntry[] {
  return Array.from({ length: count }, (_, i) =>
    makeEntry({ path: `naira/entry-${quadrant}-${ring}-${i}`, index: i, quadrant, ring }),
  );
}

describe('layoutBlips', () => {
  it('is deterministic across calls', () => {
    const entries = [...makeEntries(5, 'models', 'adopt'), ...makeEntries(5, 'knowledge', 'hold')];
    const first = layoutBlips(entries, QUADRANTS, RINGS, RADIUS);
    const second = layoutBlips(entries, QUADRANTS, RINGS, RADIUS);
    expect(first).toEqual(second);
  });

  it('numbers blips from the entry index', () => {
    const entries = makeEntries(3, 'models', 'adopt');
    const blips = layoutBlips(entries, QUADRANTS, RINGS, RADIUS);
    expect(blips.map((b) => b.number)).toEqual([1, 2, 3]);
  });

  it('keeps every blip inside its quadrant sector and ring band', () => {
    const entries = QUADRANTS.flatMap((quadrant, qi) =>
      RINGS.flatMap((ring) =>
        makeEntries(3, quadrant, ring).map((e, i) => ({
          ...e,
          path: `${e.path}-q${qi}-${i}`,
        })),
      ),
    );
    const blips = layoutBlips(entries, QUADRANTS, RINGS, RADIUS);
    const bandWidth = RADIUS / RINGS.length;

    for (const blip of blips) {
      const startAngle = quadrantStartAngle(blip.quadrantIndex);
      expect(blip.angle).toBeGreaterThanOrEqual(startAngle);
      expect(blip.angle).toBeLessThanOrEqual(startAngle + 90);

      const distance = Math.hypot(blip.x, blip.y);
      expect(distance).toBeGreaterThanOrEqual(bandWidth * blip.ringIndex);
      expect(distance).toBeLessThanOrEqual(bandWidth * (blip.ringIndex + 1) + 0.001);
    }
  });

  it('spreads colliding blips apart', () => {
    // Many entries crowded into one quadrant/ring band force collisions.
    const entries = makeEntries(8, 'agentic', 'trial');
    const blips = layoutBlips(entries, QUADRANTS, RINGS, RADIUS);

    for (let i = 0; i < blips.length; i++) {
      for (let j = i + 1; j < blips.length; j++) {
        const distance = Math.hypot(blips[i].x - blips[j].x, blips[i].y - blips[j].y);
        expect(distance).toBeGreaterThanOrEqual(BLIP_RADIUS);
      }
    }
  });
});
