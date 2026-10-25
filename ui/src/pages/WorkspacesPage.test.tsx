import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import WorkspacesPage from './WorkspacesPage';

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
      <WorkspacesPage />
    </QueryClientProvider>
  );
}

it('requires a session without fetching private data', () => {
  session.mockReturnValue({ token: null, isReady: true });
  const fetchMock = vi.fn();
  vi.stubGlobal('fetch', fetchMock);
  render(page());
  expect(screen.getByText('Sign in to manage your workspaces.')).toBeInTheDocument();
  expect(fetchMock).not.toHaveBeenCalled();
});

it('creates a workspace and registers and edits a cluster without displaying its reference', async () => {
  const user = userEvent.setup();
  let created = false;
  let cluster: Record<string, unknown> | undefined;
  const workspace = { id: 'workspace-a', name: 'Production' };
  const fetchMock = vi.fn(async (path: string, options: RequestInit) => {
    expect(options.headers).toEqual(
      expect.objectContaining({ Authorization: 'Bearer alice-token' }),
    );
    if (path === '/v1/workspaces') {
      if (options.method === 'POST') {
        expect(JSON.parse(options.body as string)).toEqual({ name: 'Production' });
        created = true;
        return Response.json(workspace, { status: 201 });
      }
      return Response.json({ workspaces: created ? [workspace] : [] });
    }
    if (options.method === 'POST' || options.method === 'PUT') {
      const input = JSON.parse(options.body as string);
      cluster = {
        id: 'cluster-a',
        workspaceId: 'workspace-a',
        name: input.name,
        kind: 'kubernetes',
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
  await screen.findByText('No workspaces yet.');
  await user.type(screen.getByLabelText('Workspace name'), 'Production');
  await user.click(screen.getByRole('button', { name: 'Create workspace' }));
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
    '/v1/workspaces/workspace-a/registrations/cluster-a',
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

it('clears the selected workspace and private data when the authenticated session changes', async () => {
  const user = userEvent.setup();
  vi.stubGlobal(
    'fetch',
    vi.fn(async (path: string, options: RequestInit) => {
      const token = (options.headers as Record<string, string>).Authorization;
      if (token === 'Bearer bob-token') return Response.json({ workspaces: [] });
      if (path === '/v1/workspaces') {
        return Response.json({
          workspaces: [{ id: 'workspace-a', name: 'Alice private workspace' }],
        });
      }
      return Response.json({
        registrations: [
          {
            id: 'cluster-a',
            name: 'Alice private cluster',
            kubernetes: { apiEndpoint: 'https://private.example.com' },
            validationStatus: 'PENDING',
          },
        ],
      });
    }),
  );
  const view = render(page());
  await user.click(await screen.findByRole('button', { name: 'Alice private workspace' }));
  await screen.findByText('https://private.example.com');
  session.mockReturnValue({ token: 'bob-token', isReady: true });
  view.rerender(page());
  expect(screen.queryByText('https://private.example.com')).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Alice private workspace' })).not.toBeInTheDocument();
  await screen.findByText('No workspaces yet.');
});

it('ignores an in-flight registration response after switching workspaces', async () => {
  const user = userEvent.setup();
  let finish: (response: Response) => void = () => {};
  let signal: AbortSignal | undefined;
  vi.stubGlobal(
    'fetch',
    vi.fn(async (path: string, options: RequestInit) => {
      if (path === '/v1/workspaces') {
        return Response.json({
          workspaces: [
            { id: 'a', name: 'A' },
            { id: 'b', name: 'B' },
          ],
        });
      }
      if (path.includes('/a/')) {
        signal = options.signal ?? undefined;
        return new Promise<Response>((resolve) => {
          finish = resolve;
        });
      }
      return Response.json({ registrations: [] });
    }),
  );
  render(page());
  await user.click(await screen.findByRole('button', { name: 'A' }));
  await screen.findByText('Loading registrations…');
  await user.click(screen.getByRole('button', { name: 'B' }));
  await screen.findByText('No clusters registered.');
  expect(signal?.aborted).toBe(true);
  await act(async () => {
    finish(Response.json({ registrations: [{ id: 'late', name: 'Late private cluster' }] }));
  });
  expect(screen.queryByText('Late private cluster')).not.toBeInTheDocument();
});

it('shows access denial instead of registration data', async () => {
  const user = userEvent.setup();
  vi.stubGlobal(
    'fetch',
    vi.fn(async (path: string) =>
      path === '/v1/workspaces'
        ? Response.json({ workspaces: [{ id: 'a', name: 'A' }] })
        : new Response('private error details', { status: 403 }),
    ),
  );
  render(page());
  await user.click(await screen.findByRole('button', { name: 'A' }));
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'You do not have access to this workspace.',
  );
  expect(screen.queryByText('private error details')).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Register cluster' })).not.toBeInTheDocument();
});
