import { useEffect, useState } from 'react';

interface AsyncDataResult<T> {
  data: T;
  loading: boolean;
  error: string | null;
}

/**
 * Generic "fetch on dependency change" hook with race-condition protection:
 * if deps change again before a request resolves, the stale response is dropped.
 */
export function useAsyncData<T>(
  loader: () => Promise<T>,
  deps: unknown[],
  initial: T,
  errorMessage: string,
): AsyncDataResult<T> {
  const [data, setData] = useState<T>(initial);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);

    loader()
      .then((result) => {
        if (cancelled) return;
        setData(result);
        setLoading(false);
      })
      .catch(() => {
        if (cancelled) return;
        setError(errorMessage);
        setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, deps);

  return { data, loading, error };
}
