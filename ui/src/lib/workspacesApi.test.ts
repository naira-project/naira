import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  createRegistration,
  createWorkspace,
  fetchRegistrations,
  fetchWorkspaces,
  updateRegistration,
} from './workspacesApi';

afterEach(() => vi.unstubAllGlobals());

describe('workspace API', () => {
  it('sends only a name when creating a workspace and authenticates every request', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response('{"id":"workspace-a","name":"A"}'));
    vi.stubGlobal('fetch', fetchMock);
    await expect(createWorkspace('token', 'A')).resolves.toEqual({ id: 'workspace-a', name: 'A' });
    expect(fetchMock).toHaveBeenCalledWith(
      '/v1/workspaces',
      expect.objectContaining({
        method: 'POST',
        headers: { Authorization: 'Bearer token', 'Content-Type': 'application/json' },
        body: '{"name":"A"}',
        cache: 'no-store',
      }),
    );
  });

  it('passes cancellation to workspace and registration queries', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(new Response('{"workspaces":[]}'))
      .mockResolvedValueOnce(new Response('{"registrations":[]}'));
    vi.stubGlobal('fetch', fetchMock);
    const { signal } = new AbortController();
    await expect(fetchWorkspaces('token', signal)).resolves.toEqual([]);
    await expect(fetchRegistrations('token', 'workspace/a', signal)).resolves.toEqual([]);
    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      '/v1/workspaces',
      expect.objectContaining({ signal, headers: { Authorization: 'Bearer token' } }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      '/v1/workspaces/workspace%2Fa/registrations',
      expect.objectContaining({ signal }),
    );
  });

  it('creates Kubernetes registrations and updates without changing kind or identity', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(new Response('{"id":"cluster-a","validationStatus":"PENDING"}'))
      .mockResolvedValueOnce(new Response(null, { status: 204 }));
    vi.stubGlobal('fetch', fetchMock);
    const input = {
      name: 'Cluster A',
      credentialRef: 'secret-reference',
      kubernetes: { apiEndpoint: 'https://kubernetes.example.com' },
    };
    await createRegistration('token', 'workspace-a', input);
    await expect(
      updateRegistration('token', 'workspace-a', 'cluster-a', input),
    ).resolves.toBeUndefined();
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({ ...input, kind: 'kubernetes' });
    expect(fetchMock).toHaveBeenLastCalledWith(
      '/v1/workspaces/workspace-a/registrations/cluster-a',
      expect.objectContaining({ method: 'PUT', body: JSON.stringify(input) }),
    );
  });

  it.each([400, 401, 403, 404, 409, 500])(
    'does not display raw error details (%s)',
    async (status) => {
      vi.stubGlobal(
        'fetch',
        vi.fn().mockResolvedValue(new Response('sensitive-error-data', { status })),
      );
      await expect(fetchWorkspaces('token')).rejects.toThrow();
      await expect(fetchWorkspaces('token')).rejects.not.toThrow('sensitive-error-data');
    },
  );
});
