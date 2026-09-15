import { useMemo, useState } from 'react';
import { Badge } from '@/components/ui/badge';
import {
  BLIP_RADIUS,
  type Blip,
  layoutBlips,
  quadrantStartAngle,
  sectorPath,
} from '@/lib/radarLayout';
import { type RadarModel, ringColor } from '@/lib/techRadar';
import MovementIndicator from './MovementIndicator';

const VIEWBOX = 780;
const CENTER = VIEWBOX / 2;
const RADIUS = CENTER - 40;

/** Corner anchors for the four quadrant labels, clockwise from top-right. */
const QUADRANT_LABELS: Array<{ x: number; y: number; anchor: 'start' | 'end' }> = [
  { x: VIEWBOX - 8, y: 20, anchor: 'end' },
  { x: VIEWBOX - 8, y: VIEWBOX - 10, anchor: 'end' },
  { x: 8, y: VIEWBOX - 10, anchor: 'start' },
  { x: 8, y: 20, anchor: 'start' },
];

interface RadarChartProps {
  model: RadarModel;
}

/**
 * The classic radar circle: quadrant sectors, ring bands, and numbered blips
 * colored by ring. Hovering a blip opens an infobox with the entry's
 * rationale and ownership.
 */
export default function RadarChart({ model }: RadarChartProps) {
  const [hovered, setHovered] = useState<Blip | null>(null);

  const quadrantIds = useMemo(() => model.quadrants.map((q) => q.id), [model.quadrants]);
  const ringIds = useMemo(() => model.rings.map((r) => r.id), [model.rings]);
  const blips = useMemo(
    () => layoutBlips(model.entries, quadrantIds, ringIds, RADIUS),
    [model.entries, quadrantIds, ringIds],
  );

  const ringCount = model.rings.length;
  const bandWidth = RADIUS / ringCount;
  const hoveredRing = hovered ? model.rings[hovered.ringIndex] : null;

  // Keep the infobox inside the chart frame: align it away from nearby edges
  // and open it upward for blips in the lower part of the chart.
  const blipX = hovered ? CENTER + hovered.x : 0;
  const blipY = hovered ? CENTER + hovered.y : 0;
  const infoboxTranslateX = blipX < 250 ? '0%' : blipX > VIEWBOX - 250 ? '-100%' : '-50%';
  const infoboxOpensUp = blipY > VIEWBOX * 0.55;

  return (
    <div className="relative w-full min-w-0 max-w-[880px] flex-1">
      <svg
        viewBox={`0 0 ${VIEWBOX} ${VIEWBOX}`}
        className="h-auto w-full"
        role="img"
        aria-label="Technology radar chart"
      >
        {/* Quadrant sector backgrounds, alternating for subtle contrast. */}
        {model.quadrants.map((quadrant, quadrantIndex) => (
          <path
            key={quadrant.id}
            d={sectorPath(
              CENTER,
              CENTER,
              0,
              RADIUS,
              quadrantStartAngle(quadrantIndex) + 1,
              quadrantStartAngle(quadrantIndex) + 89,
            )}
            fill={quadrantIndex % 2 === 0 ? '#f0fafa' : '#e6f4f4'}
          />
        ))}

        {/* Ring boundaries, outermost first so inner strokes stay visible. */}
        {model.rings.map((ring, ringIndex) => (
          <circle
            key={ring.id}
            cx={CENTER}
            cy={CENTER}
            r={bandWidth * (ringIndex + 1)}
            fill="none"
            stroke="#c9e2e4"
            strokeWidth={1.5}
          />
        ))}

        {/* Axis lines separating the quadrants. */}
        <line
          x1={CENTER - RADIUS}
          y1={CENTER}
          x2={CENTER + RADIUS}
          y2={CENTER}
          stroke="#ffffff"
          strokeWidth={8}
        />
        <line
          x1={CENTER}
          y1={CENTER - RADIUS}
          x2={CENTER}
          y2={CENTER + RADIUS}
          stroke="#ffffff"
          strokeWidth={8}
        />

        {/* Ring names along the right half of the horizontal axis. */}
        {model.rings.map((ring, ringIndex) => (
          <text
            key={ring.id}
            x={CENTER + bandWidth * (ringIndex + 0.5)}
            y={CENTER - 6}
            textAnchor="middle"
            className="fill-muted-foreground text-[11px] font-semibold uppercase tracking-wide"
          >
            {ring.name}
          </text>
        ))}

        {/* Quadrant names in the chart corners. */}
        {model.quadrants.map((quadrant, quadrantIndex) => {
          const label = QUADRANT_LABELS[quadrantIndex];
          return (
            <text
              key={quadrant.id}
              x={label.x}
              y={label.y}
              textAnchor={label.anchor}
              className="fill-foreground text-[15px] font-semibold"
            >
              {quadrant.name}
            </text>
          );
        })}

        {/* Blips. */}
        {blips.map((blip) => {
          const x = CENTER + blip.x;
          const y = CENTER + blip.y;
          const color = ringColor(blip.ringIndex);
          return (
            // biome-ignore lint/a11y/noStaticElementInteractions: hover/focus target for the infobox, not an actionable control.
            <g
              key={blip.entry.path}
              tabIndex={0}
              onMouseEnter={() => setHovered(blip)}
              onMouseLeave={() => setHovered(null)}
              onFocus={() => setHovered(blip)}
              onBlur={() => setHovered(null)}
              className="cursor-default outline-none"
            >
              <title>{`${blip.number}. ${blip.entry.name}`}</title>
              {blip.entry.moved === 'none' ? (
                <circle cx={x} cy={y} r={BLIP_RADIUS} fill={color} />
              ) : (
                <polygon
                  points={trianglePoints(BLIP_RADIUS + 2)}
                  fill={color}
                  transform={`translate(${x} ${y}) rotate(${
                    blip.entry.moved === 'in' ? blip.angle + 180 : blip.angle
                  })`}
                />
              )}
              <text
                x={x}
                y={y + 3.5}
                textAnchor="middle"
                className="pointer-events-none select-none fill-white text-[10px] font-bold"
              >
                {blip.number}
              </text>
            </g>
          );
        })}
      </svg>

      {/* Hover infobox, anchored to the blip via percentage coordinates so it
          follows the responsive SVG scaling. */}
      {hovered && (
        <div
          className="pointer-events-none absolute z-10 w-64 rounded-lg border border-gray-200 bg-card p-3 shadow-lg"
          style={{
            left: `${(blipX / VIEWBOX) * 100}%`,
            top: `${((blipY + (infoboxOpensUp ? -(BLIP_RADIUS + 6) : BLIP_RADIUS + 6)) / VIEWBOX) * 100}%`,
            transform: `translate(${infoboxTranslateX}, ${infoboxOpensUp ? '-100%' : '0'})`,
          }}
        >
          <div className="flex items-center gap-2">
            <span className="text-sm font-semibold text-foreground">
              {hovered.number}. {hovered.entry.name}
            </span>
            <MovementIndicator moved={hovered.entry.moved} />
          </div>
          <div className="mt-1.5 flex items-center gap-2">
            {hoveredRing && (
              <Badge
                className="border-transparent text-white"
                style={{ backgroundColor: ringColor(hovered.ringIndex) }}
              >
                {hoveredRing.name}
              </Badge>
            )}
            <span className="text-xs text-muted-foreground">{hovered.entry.owner}</span>
          </div>
          <p className="mt-2 text-xs leading-relaxed text-muted-foreground">
            {hovered.entry.rationale}
          </p>
        </div>
      )}
    </div>
  );
}

/** An equilateral-ish triangle pointing along the positive x-axis. */
function trianglePoints(size: number): string {
  return `${size},0 ${-size * 0.7},${-size * 0.85} ${-size * 0.7},${size * 0.85}`;
}
