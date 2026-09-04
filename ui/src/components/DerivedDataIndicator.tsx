import { TriangleAlert } from 'lucide-react';
import {
  type DerivedDetectionMethod,
  derivedDetectionDescription,
  derivedDetectionMethod,
} from '../lib/derivedData';

interface DerivedDataIndicatorProps {
  props?: Record<string, unknown> | null;
  method?: DerivedDetectionMethod | null;
  compact?: boolean;
}

/** Highlights identities derived from heuristics rather than verified metadata. */
export default function DerivedDataIndicator({
  props,
  method,
  compact = false,
}: DerivedDataIndicatorProps) {
  const detectionMethod = method ?? derivedDetectionMethod(props);
  if (!detectionMethod) {
    return null;
  }

  const description = derivedDetectionDescription(detectionMethod);
  return (
    <span
      className={
        compact
          ? 'inline-flex shrink-0 items-center ml-1 text-amber-600'
          : 'inline-flex shrink-0 items-center ml-1 gap-1 rounded-full border border-amber-300 bg-amber-50 px-2 py-0.5 text-xs font-semibold text-amber-800'
      }
      title={description}
    >
      <TriangleAlert size={compact ? 14 : 12} strokeWidth={2.5} aria-hidden="true" />
      {!compact && 'Unverified'}
    </span>
  );
}
