// Escapes s for use inside a double-quoted PromQL label value.
export function escapeLabelValue(s: string): string {
  // Backslashes first, so the ones added for quotes and newlines aren't doubled.
  return s.replace(/\\/g, '\\\\').replace(/"/g, '\\"').replace(/\n/g, '\\n');
}
