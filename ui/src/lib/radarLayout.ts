import type { RadarEntry } from './techRadar';

/**
 * Pure geometry for the radar chart. Everything here is deterministic:
 * blip positions derive from a hash of the entry path, never from Math.random,
 * so re-renders and reloads always produce the same picture.
 */

export interface Blip {
  entry: RadarEntry;
  /** 1-based display number, matching the quadrant summary cards. */
  number: number;
  x: number;
  y: number;
  /** Angle from the chart center in degrees (screen coordinates, y down). */
  angle: number;
  quadrantIndex: number;
  ringIndex: number;
}

/** Degrees kept free at both edges of each 90° quadrant sector. */
const SECTOR_PADDING_DEGREES = 8;
export const BLIP_RADIUS = 11;
const MIN_BLIP_DISTANCE = BLIP_RADIUS * 2 + 2;
const MAX_COLLISION_ITERATIONS = 40;

function toRadians(degrees: number): number {
  return (degrees * Math.PI) / 180;
}

export function polarToCartesian(
  cx: number,
  cy: number,
  radius: number,
  angleDegrees: number,
): { x: number; y: number } {
  const angle = toRadians(angleDegrees);
  return { x: cx + radius * Math.cos(angle), y: cy + radius * Math.sin(angle) };
}

/**
 * SVG path for an annular sector between two radii, sweeping clockwise from
 * startAngle to endAngle (degrees, screen coordinates). Spans must be < 180°.
 */
export function sectorPath(
  cx: number,
  cy: number,
  innerRadius: number,
  outerRadius: number,
  startAngle: number,
  endAngle: number,
): string {
  const outerStart = polarToCartesian(cx, cy, outerRadius, startAngle);
  const outerEnd = polarToCartesian(cx, cy, outerRadius, endAngle);
  const innerEnd = polarToCartesian(cx, cy, innerRadius, endAngle);
  const innerStart = polarToCartesian(cx, cy, innerRadius, startAngle);

  return [
    `M ${outerStart.x} ${outerStart.y}`,
    `A ${outerRadius} ${outerRadius} 0 0 1 ${outerEnd.x} ${outerEnd.y}`,
    `L ${innerEnd.x} ${innerEnd.y}`,
    `A ${innerRadius} ${innerRadius} 0 0 0 ${innerStart.x} ${innerStart.y}`,
    'Z',
  ].join(' ');
}

/** Quadrant index → start angle in degrees, clockwise from the top-right sector. */
export function quadrantStartAngle(quadrantIndex: number): number {
  return -90 + quadrantIndex * 90;
}

/** FNV-1a hash for deterministic pseudo-random placement. */
export function fnv1a(value: string): number {
  let hash = 0x811c9dc5;
  for (let i = 0; i < value.length; i++) {
    hash ^= value.charCodeAt(i);
    hash = Math.imul(hash, 0x01000193);
  }
  return hash >>> 0;
}

/** Maps a hash to [0, 1). */
function unitFromHash(hash: number): number {
  return (hash % 10_000) / 10_000;
}

interface SectorBounds {
  minAngle: number;
  maxAngle: number;
  minRadius: number;
  maxRadius: number;
}

function boundsFor(quadrantIndex: number, ringIndex: number, ringCount: number, radius: number) {
  const startAngle = quadrantStartAngle(quadrantIndex);
  const bandWidth = radius / ringCount;
  const margin = Math.min(BLIP_RADIUS + 2, bandWidth / 2);
  const bounds: SectorBounds = {
    minAngle: startAngle + SECTOR_PADDING_DEGREES,
    maxAngle: startAngle + 90 - SECTOR_PADDING_DEGREES,
    minRadius: bandWidth * ringIndex + margin,
    maxRadius: bandWidth * (ringIndex + 1) - margin,
  };
  // The innermost band leaves extra room so blips never sit on the center.
  if (ringIndex === 0) {
    bounds.minRadius = Math.max(bounds.minRadius, BLIP_RADIUS + 4);
  }
  if (bounds.minRadius > bounds.maxRadius) {
    const middle = bandWidth * (ringIndex + 0.5);
    bounds.minRadius = middle;
    bounds.maxRadius = middle;
  }
  return bounds;
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), max);
}

function place(blip: Blip, bounds: SectorBounds, angle: number, radius: number) {
  blip.angle = clamp(angle, bounds.minAngle, bounds.maxAngle);
  const clampedRadius = clamp(radius, bounds.minRadius, bounds.maxRadius);
  const { x, y } = polarToCartesian(0, 0, clampedRadius, blip.angle);
  blip.x = x;
  blip.y = y;
}

/**
 * Positions every entry inside its quadrant sector and ring band around the
 * origin (translate by the chart center when rendering). Overlapping blips are
 * nudged apart deterministically along their sector arc.
 */
export function layoutBlips(
  entries: RadarEntry[],
  quadrantIds: string[],
  ringIds: string[],
  radius: number,
): Blip[] {
  const blips: Blip[] = entries.map((entry) => {
    const quadrantIndex = Math.max(quadrantIds.indexOf(entry.quadrant), 0);
    const ringIndex = Math.max(ringIds.indexOf(entry.ring), 0);
    const bounds = boundsFor(quadrantIndex, ringIndex, ringIds.length, radius);

    const angleUnit = unitFromHash(fnv1a(entry.path));
    const radiusUnit = unitFromHash(fnv1a(`${entry.path}#radius`));
    const blip: Blip = {
      entry,
      number: entry.index + 1,
      x: 0,
      y: 0,
      angle: 0,
      quadrantIndex,
      ringIndex,
    };
    place(
      blip,
      bounds,
      bounds.minAngle + angleUnit * (bounds.maxAngle - bounds.minAngle),
      bounds.minRadius + radiusUnit * (bounds.maxRadius - bounds.minRadius),
    );
    return blip;
  });

  resolveCollisions(blips, ringIds.length, radius);
  return blips;
}

function resolveCollisions(blips: Blip[], ringCount: number, radius: number) {
  // Bounds and sweep direction are invariant per blip; hoist them out of the
  // O(iterations · n²) pair loop instead of recomputing (and re-hashing the
  // entry path) on every collision.
  const blipBounds = blips.map((blip) =>
    boundsFor(blip.quadrantIndex, blip.ringIndex, ringCount, radius),
  );
  const blipDirection = blips.map((blip) => (fnv1a(blip.entry.path) % 2 === 0 ? 1 : -1));

  // Push one blip away from the other along their separation vector, clamped
  // back into the blip's own quadrant/ring bounds. Repulsion moves blips both
  // radially and angularly, so crowds near the center — where the sector arc
  // is shorter than a blip diameter — can still spread across the band.
  const push = (blip: Blip, index: number, awayFrom: Blip, distance: number) => {
    const strength = (MIN_BLIP_DISTANCE - distance) / 2 + 0.5;
    let dx = (blip.x - awayFrom.x) / (distance || 1);
    let dy = (blip.y - awayFrom.y) / (distance || 1);
    if (distance < 0.001) {
      // Coincident blips: break the tie with the deterministic per-blip sweep
      // direction instead of a zero-length repulsion vector.
      dx = 0;
      dy = blipDirection[index];
    }
    const nx = blip.x + dx * strength;
    const ny = blip.y + dy * strength;
    const bounds = blipBounds[index];
    // atan2 yields (-180, 180]; normalize into the bounds' period so sectors
    // beyond 180° (quadrant 3 spans 180–270°) clamp correctly.
    const rawAngle = (Math.atan2(ny, nx) * 180) / Math.PI;
    const angle = ((((rawAngle - bounds.minAngle) % 360) + 360) % 360) + bounds.minAngle;
    place(blip, bounds, angle, Math.hypot(nx, ny));
  };

  for (let iteration = 0; iteration < MAX_COLLISION_ITERATIONS; iteration++) {
    let moved = false;
    for (let i = 0; i < blips.length; i++) {
      for (let j = i + 1; j < blips.length; j++) {
        const a = blips[i];
        const b = blips[j];
        const distance = Math.hypot(a.x - b.x, a.y - b.y);
        if (distance >= MIN_BLIP_DISTANCE) {
          continue;
        }

        push(a, i, b, distance);
        push(b, j, a, distance);
        moved = true;
      }
    }
    if (!moved) {
      return;
    }
  }
}
