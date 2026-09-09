const DERIVED_DETECTION_METHODS = new Set(['INFERRED', 'OCI_STANDARD']);

export type DerivedDetectionMethod = 'INFERRED' | 'OCI_STANDARD';

export function derivedDetectionMethod(
  props: Record<string, unknown> | null | undefined,
): DerivedDetectionMethod | null {
  const method = props?.detection_method;
  return typeof method === 'string' && DERIVED_DETECTION_METHODS.has(method)
    ? (method as DerivedDetectionMethod)
    : null;
}

export function derivedDetectionDescription(method: DerivedDetectionMethod) {
  if (method === 'OCI_STANDARD') {
    return 'Source identity was derived from the OCI image naming convention and is not verified.';
  } else if (method === 'INFERRED') {
    return 'Source identity was inferred from available data and is not verified.';
  }
  return '';
}
