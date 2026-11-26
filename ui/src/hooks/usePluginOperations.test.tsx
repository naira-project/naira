import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';
import type { PropsWithChildren } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fetchOperations } from '../lib/catalogApi';
import { queryKeys } from '../lib/queryKeys';
import { useOperationsSync } from './usePluginOperations';

vi.mock('../lib/catalogApi', () => ({
  fetchOperations: vi.fn(),
  fetchPlugins: vi.fn(),
  runAllPlugins: vi.fn(),
  runPlugin: vi.fn(),
}));

vi.mock('./useOpenMFPContext', () => ({
  useOpenMFPContext: () => ({ token: null, userId: null, userEmail: null, isReady: true }),
}));

const fetchOperationsMock = vi.mocked(fetchOperations);

function createOperation(overrides: {
  done: boolean;
  error?: { message: string };
  plugin?: string;
}) {
  return {
    name: 'operations/run-1',
    done: overrides.done,
    metadata: {
      plugin: overrides.plugin ?? 'plugin-a',
      startTime: '2026-01-01T00:00:00.000Z',
      createdAt: '2026-01-01T00:00:00.000Z',
      ...(overrides.done ? { endTime: '2026-01-01T00:00:01.000Z' } : {}),
    },
    ...(overrides.error ? { error: overrides.error } : {}),
    ...(overrides.done && !overrides.error
      ? { response: { nodesUpserted: 1, relationsUpserted: 0 } }
      : {}),
  };
}

function createWrapper(queryClient: QueryClient) {
  return function Wrapper({ children }: PropsWithChildren) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
  };
}

describe('useOperationsSync', () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false, staleTime: Infinity, gcTime: Infinity },
      },
    });
    fetchOperationsMock.mockResolvedValue([]);
  });

  afterEach(() => {
    queryClient.clear();
    vi.clearAllMocks();
  });

  it('records completed operations on the initial load without invalidating catalog data', async () => {
    fetchOperationsMock.mockResolvedValue([createOperation({ done: true })]);
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries');

    const { result } = renderHook(() => useOperationsSync(), {
      wrapper: createWrapper(queryClient),
    });

    await waitFor(() => expect(result.current.data).toHaveLength(1));
    expect(invalidate).not.toHaveBeenCalled();
  });

  it('invalidates catalog data when an observed run completes successfully', async () => {
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries');
    const { result } = renderHook(() => useOperationsSync(), {
      wrapper: createWrapper(queryClient),
    });

    await waitFor(() => expect(result.current.data).toEqual([]));
    act(() => {
      queryClient.setQueryData(queryKeys.operations, [createOperation({ done: false })]);
    });
    await waitFor(() => expect(result.current.data?.[0]?.done).toBe(false));

    act(() => {
      queryClient.setQueryData(queryKeys.operations, [createOperation({ done: true })]);
    });

    await waitFor(() => expect(invalidate).toHaveBeenCalledWith({ queryKey: queryKeys.catalog }));
  });

  it('does not invalidate catalog data when a run fails', async () => {
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries');
    const { result } = renderHook(() => useOperationsSync(), {
      wrapper: createWrapper(queryClient),
    });

    await waitFor(() => expect(result.current.data).toEqual([]));
    act(() => {
      queryClient.setQueryData(queryKeys.operations, [createOperation({ done: false })]);
    });
    await waitFor(() => expect(result.current.data?.[0]?.done).toBe(false));

    act(() => {
      queryClient.setQueryData(queryKeys.operations, [
        createOperation({ done: true, error: { message: 'run failed' } }),
      ]);
    });

    await waitFor(() => expect(result.current.data?.[0]?.done).toBe(true));
    expect(invalidate).not.toHaveBeenCalled();
  });

  it('invalidates catalog data when an operation seen running disappears', async () => {
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries');
    const { result } = renderHook(() => useOperationsSync(), {
      wrapper: createWrapper(queryClient),
    });

    await waitFor(() => expect(result.current.data).toEqual([]));
    act(() => {
      queryClient.setQueryData(queryKeys.operations, [createOperation({ done: false })]);
    });
    await waitFor(() => expect(result.current.data?.[0]?.done).toBe(false));

    act(() => {
      queryClient.setQueryData(queryKeys.operations, []);
    });

    await waitFor(() => expect(invalidate).toHaveBeenCalledWith({ queryKey: queryKeys.catalog }));
  });
});
