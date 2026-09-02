import { useNavigate } from 'react-router';
import { Card, CardContent } from '@/components/ui/card';
import type { RadarEntry, RadarModel, RadarQuadrant } from '@/lib/techRadar';
import { ringColor } from '@/lib/techRadar';
import MovementIndicator from './MovementIndicator';

interface QuadrantSummaryCardProps {
  quadrant: RadarQuadrant;
  model: RadarModel;
}

/**
 * Per-quadrant card listing the quadrant's entries grouped by ring, with the
 * blip numbers from the chart and a link to the quadrant detail view.
 */
export default function QuadrantSummaryCard({ quadrant, model }: QuadrantSummaryCardProps) {
  const navigate = useNavigate();
  const entries = model.entries.filter((entry) => entry.quadrant === quadrant.id);

  const byRing = model.rings
    .map((ring, ringIndex) => ({
      ring,
      ringIndex,
      entries: entries.filter((entry) => entry.ring === ring.id),
    }))
    .filter((group) => group.entries.length > 0);

  return (
    <Card>
      <CardContent>
        <div className="flex items-baseline justify-between">
          <h3 className="text-sm font-semibold text-foreground">{quadrant.name}</h3>
          <span className="text-xs text-muted-foreground">
            {entries.length} {entries.length === 1 ? 'entry' : 'entries'}
          </span>
        </div>

        {entries.length === 0 ? (
          <p className="mt-3 text-xs text-muted-foreground">No entries in this quadrant.</p>
        ) : (
          <div className="mt-3 flex flex-col gap-3">
            {byRing.map(({ ring, ringIndex, entries: ringEntries }) => (
              <div key={ring.id}>
                <span
                  className="text-[0.65rem] font-semibold uppercase tracking-wide"
                  style={{ color: ringColor(ringIndex) }}
                >
                  {ring.name}
                </span>
                <ul className="mt-1 flex flex-col gap-0.5">
                  {ringEntries.map((entry: RadarEntry) => (
                    <li key={entry.path} className="flex items-center gap-1.5 text-sm">
                      <span className="w-6 shrink-0 text-right text-xs text-muted-foreground">
                        {entry.index + 1}.
                      </span>
                      <span className="truncate text-foreground" title={entry.name}>
                        {entry.name}
                      </span>
                      <MovementIndicator moved={entry.moved} />
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </div>
        )}
      </CardContent>
      <div className="px-4 pb-4">
        <button
          type="button"
          onClick={() => navigate(`/tech-radar/${encodeURIComponent(quadrant.id)}`)}
          className="flex w-full items-center justify-center rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-white transition-opacity hover:opacity-90"
        >
          View details
        </button>
      </div>
    </Card>
  );
}
