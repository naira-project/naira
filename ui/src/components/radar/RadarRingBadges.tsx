import { Radar } from 'lucide-react';
import { Link } from 'react-router';
import { Badge } from '@/components/ui/badge';
import { useTechRadar } from '@/hooks/useTechRadar';
import type { NodeResource } from '@/lib/catalogApi';
import { entriesMatchingNode, ringColor } from '@/lib/techRadar';

/**
 * Shows the tech radar ring of every radar entry whose catalog refs match
 * `node`, each linking to the entry's quadrant. Renders nothing while the
 * radar loads, when no radar is synced, or when no entry matches.
 */
export default function RadarRingBadges({ node }: { node: NodeResource }) {
  const { model } = useTechRadar();
  if (!model) {
    return null;
  }
  const entries = entriesMatchingNode(model.entries, node);
  if (entries.length === 0) {
    return null;
  }

  const ringIndexById = new Map(model.rings.map((ring, index) => [ring.id, index]));
  const ringNameById = new Map(model.rings.map((ring) => [ring.id, ring.name]));
  return (
    <div className="flex flex-wrap items-center gap-2">
      {entries.map((entry) => (
        <Link
          key={entry.path}
          to={`/tech-radar/${encodeURIComponent(entry.quadrant)}`}
          title="Open in the tech radar"
        >
          <Badge
            className="gap-1 border-transparent text-white"
            style={{ backgroundColor: ringColor(ringIndexById.get(entry.ring) ?? 0) }}
          >
            <Radar size={12} aria-hidden />
            {entry.name} · {ringNameById.get(entry.ring) ?? entry.ring}
          </Badge>
        </Link>
      ))}
    </div>
  );
}
