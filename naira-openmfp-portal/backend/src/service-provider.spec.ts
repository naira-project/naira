import assert from 'node:assert/strict';
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { after, describe, it } from 'node:test';
import { Logger } from '@nestjs/common';
import type { LuigiNode, RawServiceProvider } from '@openmfp/portal-server-lib';
import { serviceProviders } from './service-provider.js';

// The invalid files below make the portal log an error each time.
Logger.overrideLogger(false);

const dir = mkdtempSync(path.join(tmpdir(), 'service-provider-'));
after(() => rmSync(dir, { recursive: true, force: true }));

const nodesOf = (providers: RawServiceProvider[]): LuigiNode[] =>
  providers.flatMap((p) => p.contentConfiguration.flatMap((c) => c.luigiConfigFragment.data.nodes));
const category = (node: LuigiNode) => (typeof node.category === 'string' ? node.category : node.category?.id);

const STATIC_SEGMENTS = [
  'naira',
  'overview',
  'plugins',
  'model',
  'mcp',
  'dataset',
  'inference_endpoints',
  'software_catalog',
  'tech-radar',
  'coming-soon',
];

const wikiNode = (id: string, order: number): LuigiNode => ({
  pathSegment: id,
  label: id,
  order,
  category: { id: 'docs', label: 'Docs' },
  entityType: 'naira',
  url: `http://${id}.wiki.localhost:3002`,
  virtualTree: true,
});

describe('serviceProviders', () => {
  it('has the portal pages and no wiki node when there are no wikis', () => {
    assert.deepEqual(nodesOf(serviceProviders([])).map((n) => n.pathSegment), STATIC_SEGMENTS);
  });

  it('no longer has the placeholder wiki node, which pointed at a page the UI does not have', () => {
    assert.ok(!nodesOf(serviceProviders([])).some((n) => n.pathSegment === 'wiki'));
  });

  it('has no Docs group without wikis, because Luigi builds groups from node categories', () => {
    assert.ok(!nodesOf(serviceProviders([])).some((n) => category(n) === 'docs'));
  });

  it('keeps the order of the portal pages', () => {
    const orders = nodesOf(serviceProviders([])).filter((n) => n.order !== undefined).map((n) => n.order);
    assert.deepEqual(orders, [0, 1, 2, 3, 4, 5, 6, 7, 8]);
  });

  it('appends wiki nodes after the portal pages and leaves those unchanged', () => {
    const nodes = nodesOf(serviceProviders([wikiNode('handbook', 9), wikiNode('runbooks', 10)]));
    assert.deepEqual(nodes.slice(0, STATIC_SEGMENTS.length), nodesOf(serviceProviders([])));
    assert.deepEqual(nodes.slice(STATIC_SEGMENTS.length).map((n) => n.pathSegment), ['handbook', 'runbooks']);
  });

  it('puts the wikis in the Docs group', () => {
    const docs = nodesOf(serviceProviders([wikiNode('handbook', 9)])).filter((n) => category(n) === 'docs');
    assert.deepEqual(docs.map((n) => n.pathSegment), ['handbook']);
  });
});

describe('SERVICE_PROVIDERS', () => {
  // A query string gives each import its own module instance, so each reads its own environment.
  let n = 0;
  async function providersFor(wikisFile: string | undefined): Promise<RawServiceProvider[]> {
    if (wikisFile === undefined) delete process.env.NAIRA_WIKIS_FILE;
    else process.env.NAIRA_WIKIS_FILE = wikisFile;
    const module = await import(`./service-provider.js?case=${n++}`);
    return module.SERVICE_PROVIDERS;
  }
  const wikisFile = (content: unknown) => {
    const name = path.join(dir, `${n}.json`);
    writeFileSync(name, JSON.stringify(content));
    return name;
  };
  const wiki = (id: string, extra = {}) => ({ id, label: id, url: `http://${id}.wiki.localhost:3002/`, primary: false, order: 0, ...extra });

  it('has the portal pages only when NAIRA_WIKIS_FILE is not set', async () => {
    assert.deepEqual(nodesOf(await providersFor(undefined)).map((n) => n.pathSegment), STATIC_SEGMENTS);
  });

  it('adds the wikis of NAIRA_WIKIS_FILE after the last portal page, primary first', async () => {
    const providers = await providersFor(wikisFile([wiki('second', { order: 1 }), wiki('first', { primary: true, order: 5 })]));
    const added = nodesOf(providers).slice(STATIC_SEGMENTS.length);
    assert.deepEqual(added.map((x) => [x.pathSegment, x.order, x.url]), [
      ['first', 9, 'http://first.wiki.localhost:3002'],
      ['second', 10, 'http://second.wiki.localhost:3002'],
    ]);
  });

  it('adds nothing for an empty list', async () => {
    assert.deepEqual(nodesOf(await providersFor(wikisFile([]))).map((x) => x.pathSegment), STATIC_SEGMENTS);
  });

  it('keeps every portal page and adds no wiki when the file is invalid', async () => {
    assert.deepEqual(nodesOf(await providersFor(path.join(dir, 'missing.json'))).map((x) => x.pathSegment), STATIC_SEGMENTS);
  });

  for (const segment of ['overview', 'plugins', 'model', 'mcp', 'dataset', 'inference_endpoints', 'software_catalog', 'tech-radar', 'coming-soon', 'naira']) {
    it(`refuses a wiki called ${segment}, which would shadow a portal page`, async () => {
      const nodes = nodesOf(await providersFor(wikisFile([wiki('fine'), wiki(segment)])));
      assert.deepEqual(nodes.map((x) => x.pathSegment), STATIC_SEGMENTS);
    });
  }
});
