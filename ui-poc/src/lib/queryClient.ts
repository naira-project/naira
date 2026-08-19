import { QueryClient } from '@tanstack/react-query';

/**
 * Single shared QueryClient for the app.
 * staleTime is intentionally generous: catalog data changes only after a
 * plugin run, which explicitly invalidates the relevant queries, so we
 * don't need aggressive background refetching.
 */
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
});
