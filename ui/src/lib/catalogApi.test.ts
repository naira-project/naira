import { afterEach, describe, expect, it, vi } from 'vitest';
import { fetchKinds } from './catalogApi';

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('fetchKinds', () => {
  it('fetches every page of node kinds with authentication', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response('{"kinds":["application"],"nextPageToken":"next-token"}'))
      .mockResolvedValueOnce(new Response('{"kinds":["model"]}'));
    vi.stubGlobal('fetch', fetchMock);

    await expect(fetchKinds('token')).resolves.toEqual(['application', 'model']);
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(fetchMock).toHaveBeenNthCalledWith(1, '/v1/kinds?pageSize=1000', {
      headers: { Authorization: 'Bearer token' },
    });
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/v1/kinds?pageSize=1000&pageToken=next-token', {
      headers: { Authorization: 'Bearer token' },
    });
  });

  it('returns an empty list for an empty catalog', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('{"kinds":[],"totalSize":0}')));
    await expect(fetchKinds(null)).resolves.toEqual([]);
  });

  it('propagates errors from subsequent pages', async () => {
    vi.stubGlobal('fetch', vi.fn()
      .mockResolvedValueOnce(new Response('{"kinds":["model"],"nextPageToken":"next-token"}'))
      .mockResolvedValueOnce(new Response(null, { status: 500 })));
    await expect(fetchKinds(null)).rejects.toThrow('Request failed');
  });
});