import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"
import { OperationResource } from './catalogApi';

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

export function formatRelativeTime(iso: string): string {
  const then = new Date(iso).getTime();
  const diffMs = Date.now() - then;
  const seconds = Math.max(0, Math.floor(diffMs / 1000));

  if (seconds < 60) return `just now`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes} min ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

/**
 * Returns the most recent operation from a list of operations.
 */
export function latestOperation(operations: OperationResource[]): OperationResource | null {
  if (operations.length === 0) return null;
  return [...operations].sort(
    (a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime()
  )[0];
}

/**
 * Returns a map of plugin name → most recent operation for each plugin.
 */
export function latestOperationPerPlugin(
  operations: OperationResource[]
): Map<string, OperationResource> {
  const sorted = [...operations].sort(
    (a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime()
  );
  const latest = new Map<string, OperationResource>();
  for (const op of sorted) {
    if (!latest.has(op.plugin)) {
      latest.set(op.plugin, op);
    }
  }
  return latest;
}
