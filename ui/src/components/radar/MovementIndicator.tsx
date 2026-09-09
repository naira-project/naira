import { MoveDownRight, MoveUpRight } from 'lucide-react';
import type { RadarMovement } from '@/lib/techRadar';

/**
 * Inline marker for an entry's movement since the previous edition:
 * "in" = moved toward adoption, "out" = moved away from it.
 */
export default function MovementIndicator({ moved }: { moved: RadarMovement }) {
  if (moved === 'in') {
    return (
      <span
        className="inline-flex items-center gap-0.5 text-xs font-medium text-green-600"
        title="Moved in since the previous edition"
      >
        <MoveUpRight size={12} />
        in
      </span>
    );
  }
  if (moved === 'out') {
    return (
      <span
        className="inline-flex items-center gap-0.5 text-xs font-medium text-red-600"
        title="Moved out since the previous edition"
      >
        <MoveDownRight size={12} />
        out
      </span>
    );
  }
  return null;
}
