import type { RadarRing } from '@/lib/techRadar';
import { ringColor } from '@/lib/techRadar';

/**
 * The ring definitions from the radar config, innermost ring first.
 */
export default function RingLegend({ rings }: { rings: RadarRing[] }) {
  return (
    <div className="flex flex-col gap-2">
      {rings.map((ring, ringIndex) => (
        <div key={ring.id} className="flex items-start gap-2">
          <span
            className="mt-1 size-2.5 shrink-0 rounded-full"
            style={{ backgroundColor: ringColor(ringIndex) }}
          />
          <div>
            <span className="text-xs font-semibold text-foreground">{ring.name}</span>
            {ring.description && (
              <p className="text-[11px] leading-snug text-muted-foreground">{ring.description}</p>
            )}
          </div>
        </div>
      ))}
    </div>
  );
}
