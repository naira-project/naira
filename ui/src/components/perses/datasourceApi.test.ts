import { describe, expect, it } from 'vitest';
import { createDatasourceApi } from './datasourceApi';

async function prometheusSpec(token: string | null) {
  const [datasource] = await createDatasourceApi(token).listGlobalDatasources!();
  return datasource.spec.plugin.spec as Record<string, unknown>;
}

describe('createDatasourceApi', () => {
  it('sends the token as a Bearer Authorization header on the proxy', async () => {
    expect(await prometheusSpec('abc.def.ghi')).toEqual({
      directUrl: '/prometheus',
      proxy: {
        kind: 'HTTPProxy',
        spec: { url: '/prometheus', headers: { Authorization: 'Bearer abc.def.ghi' } },
      },
    });
  });

  it.each([{ name: 'null', token: null }, { name: 'empty', token: '' }])(
    'omits the proxy and its header when the token is $name',
    async ({ token }) => {
      expect(await prometheusSpec(token)).toEqual({ directUrl: '/prometheus' });
    },
  );

  it('exposes the datasource as the global default named prometheus', async () => {
    const [datasource] = await createDatasourceApi('t').listGlobalDatasources!();
    expect(datasource.metadata.name).toBe('prometheus');
    expect(datasource.spec.default).toBe(true);
    expect(datasource.spec.plugin.kind).toBe('PrometheusDatasource');
  });
});
