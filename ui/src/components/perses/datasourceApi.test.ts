import { describe, expect, it } from 'vitest';
import { createDatasourceApi } from './datasourceApi';

async function globalDatasource(token: string | null) {
  const datasource = await createDatasourceApi(token).getGlobalDatasource({
    kind: 'PrometheusDatasource',
  });
  if (!datasource) throw new Error('global datasource not found');
  return datasource;
}

describe('createDatasourceApi', () => {
  it('sends the token as a Bearer Authorization header on the proxy', async () => {
    const datasource = await globalDatasource('abc.def.ghi');
    expect(datasource.spec.plugin.spec).toEqual({
      directUrl: '/prometheus',
      proxy: {
        kind: 'HTTPProxy',
        spec: { url: '/prometheus', headers: { Authorization: 'Bearer abc.def.ghi' } },
      },
    });
  });

  it.each([
    { name: 'null', token: null },
    { name: 'empty', token: '' },
  ])('omits the proxy and its header when the token is $name', async ({ token }) => {
    const datasource = await globalDatasource(token);
    expect(datasource.spec.plugin.spec).toEqual({ directUrl: '/prometheus' });
  });

  it('exposes the datasource as the global default named prometheus', async () => {
    const datasource = await globalDatasource('t');
    expect(datasource.metadata.name).toBe('prometheus');
    expect(datasource.spec.default).toBe(true);
    expect(datasource.spec.plugin.kind).toBe('PrometheusDatasource');
  });
});
