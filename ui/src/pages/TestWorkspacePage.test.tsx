import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import TestWorkspacePage from './TestWorkspacePage';

const session = vi.hoisted(() => vi.fn());
vi.mock('../hooks/useOpenMFPContext', () => ({ useOpenMFPContext: session }));

let client: QueryClient;
beforeEach(() => {
  session.mockReturnValue({ token: 'alice-token', isReady: true });
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
});
afterEach(() => {
  cleanup();
  client.clear();
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

function page() {
  return (
    <QueryClientProvider client={client}>
      <TestWorkspacePage />
    </QueryClientProvider>
  );
}

it('requires a session without fetching cluster data', () => {
  session.mockReturnValue({ token: null, isReady: true });
  const fetchMock = vi.fn();
  vi.stubGlobal('fetch', fetchMock);
  render(page());
  expect(screen.getByText('Sign in to manage clusters in test workspace.')).toBeInTheDocument();
  expect(fetchMock).not.toHaveBeenCalled();
});

it('shows the fixed workspace directly and registers and edits clusters without workspace management', async () => {
  const user = userEvent.setup();
  let cluster: Record<string, unknown> | undefined;
  const fetchMock = vi.fn(async (path: string, options: RequestInit) => {
    expect(path).toMatch(/^\/v1\/workspaces\/test-workspace\/registrations(?:\/cluster-a)?$/);
    expect(options.headers).toEqual(
      expect.objectContaining({ Authorization: 'Bearer alice-token' }),
    );
    if (options.method === 'POST' || options.method === 'PUT') {
      const input = JSON.parse(options.body as string);
      cluster = {
        id: 'cluster-a',
        workspaceId: 'test-workspace',
        name: input.name,
        kubernetes: input.kubernetes,
        validationStatus: 'PENDING',
      };
      return options.method === 'PUT'
        ? new Response(null, { status: 204 })
        : Response.json(cluster, { status: 201 });
    }
    return Response.json({ registrations: cluster ? [cluster] : [] });
  });
  vi.stubGlobal('fetch', fetchMock);
  render(page());
  expect(screen.getByRole('heading', { name: 'test workspace' })).toBeInTheDocument();
  expect(screen.queryByLabelText('Workspace name')).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: /workspace/i })).not.toBeInTheDocument();
  await screen.findByText('No clusters registered.');
  await user.type(screen.getByLabelText('Cluster name'), 'Cluster A');
  await user.type(screen.getByLabelText('Kubernetes API endpoint'), 'https://cluster.example.com');
  await user.type(screen.getByLabelText(/Credential reference/), 'private-reference');
  await user.click(screen.getByRole('button', { name: 'Register cluster' }));
  await screen.findByRole('heading', { name: 'Cluster A' });
  expect(screen.getByText(/PENDING/)).toBeInTheDocument();
  await waitFor(() => expect(screen.getByLabelText(/Credential reference/)).toHaveValue(''));
  expect(screen.queryByText('private-reference')).not.toBeInTheDocument();

  await user.click(screen.getByRole('button', { name: 'Edit Cluster A' }));
  await user.clear(screen.getByLabelText('Cluster name'));
  await user.type(screen.getByLabelText('Cluster name'), 'Renamed cluster');
  await user.type(screen.getByLabelText(/Credential reference/), 'replacement-reference');
  await user.click(screen.getByRole('button', { name: 'Save registration' }));
  await screen.findByRole('heading', { name: 'Renamed cluster' });
  expect(fetchMock).toHaveBeenCalledWith(
    '/v1/workspaces/test-workspace/registrations/cluster-a',
    expect.objectContaining({ method: 'PUT' }),
  );
  expect(
    JSON.stringify(
      client
        .getQueryCache()
        .getAll()
        .map((query) => query.queryKey),
    ),
  ).not.toContain('alice-token');
});

it('shows the same workspace after changing users without reusing cached session data', async () => {
  const fetchMock = vi.fn(async (_path: string, options: RequestInit) => {
    const token = (options.headers as Record<string, string>).Authorization;
    if (token === 'Bearer bob-token') return Response.json({ registrations: [] });
    return Response.json({
      registrations: [
        {
          id: 'cluster-a',
          name: 'Shared cluster',
          kubernetes: { apiEndpoint: 'https://cluster.example.com' },
          validationStatus: 'PENDING',
        },
      ],
    });
  });
  vi.stubGlobal('fetch', fetchMock);
  const view = render(page());
  await screen.findByText('https://cluster.example.com');
  session.mockReturnValue({ token: 'bob-token', isReady: true });
  view.rerender(page());
  expect(screen.queryByText('https://cluster.example.com')).not.toBeInTheDocument();
  expect(screen.getByRole('heading', { name: 'test workspace' })).toBeInTheDocument();
  await screen.findByText('No clusters registered.');
  expect(fetchMock).toHaveBeenLastCalledWith(
    '/v1/workspaces/test-workspace/registrations',
    expect.objectContaining({ headers: { Authorization: 'Bearer bob-token' } }),
  );
});

it('cancels a pending request when signed out and does not render its late response', async () => {
  let finish: (response: Response) => void = () => {};
  let signal: AbortSignal | undefined;
  vi.stubGlobal(
    'fetch',
    vi.fn((_path: string, options: RequestInit) => {
      signal = options.signal ?? undefined;
      return new Promise<Response>((resolve) => {
        finish = resolve;
      });
    }),
  );
  const view = render(page());
  await screen.findByText('Loading registrations…');
  session.mockReturnValue({ token: null, isReady: true });
  view.rerender(page());
  expect(signal?.aborted).toBe(true);
  await act(async () => {
    finish(Response.json({ registrations: [{ id: 'late', name: 'Late cluster' }] }));
  });
  expect(screen.queryByText('Late cluster')).not.toBeInTheDocument();
  expect(screen.getByText('Sign in to manage clusters in test workspace.')).toBeInTheDocument();
});

it('shows safe request errors without exposing a registration form', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(new Response('sensitive error details', { status: 500 })),
  );
  render(page());
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'The request failed. Please try again.',
  );
  expect(screen.queryByText('sensitive error details')).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Register cluster' })).not.toBeInTheDocument();
});

it('tests a saved cluster only on request and shows per-check results', async () => {
  const user = userEvent.setup();
  let result: Record<string, unknown> | undefined;
  let finish: ((response: Response) => void) | undefined;
  const fetchMock = vi.fn(async (path: string, options: RequestInit) => {
    if (path.endsWith('/test')) {
      expect(options.method).toBe('POST');
      return new Promise<Response>((resolve) => {
        finish = resolve;
      });
    }
    return Response.json({
      registrations: [
        {
          id: 'cluster-a',
          name: 'Cluster A',
          kubernetes: { apiEndpoint: 'https://cluster.example.com' },
          validationStatus: result ? 'FAILED' : 'PENDING',
          lastValidation: result,
        },
      ],
    });
  });
  vi.stubGlobal('fetch', fetchMock);
  render(page());
  await screen.findByRole('heading', { name: 'Cluster A' });
  expect(fetchMock).toHaveBeenCalledTimes(1);
  await user.click(screen.getByRole('button', { name: 'Test connection Cluster A' }));
  expect(screen.getByRole('button', { name: 'Test connection Cluster A' })).toBeDisabled();
  expect(screen.getByRole('button', { name: 'Edit Cluster A' })).toBeDisabled();
  expect(screen.getByText('Testing…')).toBeInTheDocument();
  result = {
    status: 'FAILED',
    checkedAt: '2026-10-26T12:00:00Z',
    checks: [
      { name: 'connection', passed: true, code: 'OK', message: 'Connected securely.' },
      {
        name: 'services',
        passed: false,
        code: 'FORBIDDEN',
        message: 'Missing Services read access.',
      },
    ],
  };
  await act(async () => {
    finish?.(Response.json(result));
  });
  const results = await screen.findByRole('region', { name: 'Connection results Cluster A' });
  expect(results).toHaveTextContent('FAIL — services');
  expect(results).toHaveTextContent('Missing Services read access.');
  expect(results).toHaveTextContent('PASS — connection');
  await waitFor(() => expect(screen.getByRole('button', { name: 'Edit Cluster A' })).toBeEnabled());
});
