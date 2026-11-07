import assert from 'node:assert/strict';
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { after, describe, it } from 'node:test';
import { loadWikiNodes, parseWikis, wikiNodes, type Wiki } from './wikis.js';

const dir = mkdtempSync(path.join(tmpdir(), 'wikis-'));
after(() => rmSync(dir, { recursive: true, force: true }));

const DOCS = { id: 'docs', label: 'Docs', isGroup: true };
const RESERVED = new Set(['overview', 'plugins']);

function entry(id: string, extra: Record<string, unknown> = {}) {
  return { id, label: id, url: `http://${id}.wiki.localhost:3002/`, primary: false, order: 0, ...extra };
}

function file(content: unknown): string {
  const name = path.join(dir, `${Math.random().toString(36).slice(2)}.json`);
  writeFileSync(name, typeof content === 'string' ? content : JSON.stringify(content));
  return name;
}

function load(filePath: string | undefined) {
  const errors: string[] = [];
  const nodes = loadWikiNodes(filePath, {
    category: DOCS,
    firstOrder: 9,
    reservedIds: RESERVED,
    log: (message) => errors.push(message),
  });
  return { nodes, errors };
}

describe('parseWikis', () => {
  it('reads the fields the chart renders and ignores the rest', () => {
    const wikis = parseWikis(
      JSON.stringify([entry('handbook', { label: 'Platform Handbook', owner: 'team', source: 'https://x', extra: 1 })]),
      RESERVED,
    );
    assert.deepEqual(wikis, [
      { id: 'handbook', label: 'Platform Handbook', url: 'http://handbook.wiki.localhost:3002/', primary: false, order: 0 },
    ]);
  });

  it('accepts an empty list', () => {
    assert.deepEqual(parseWikis('[]', RESERVED), []);
  });

  it('defaults primary to false and order to 0', () => {
    const [wiki] = parseWikis(JSON.stringify([{ id: 'a', label: 'A', url: 'http://a.example/' }]), RESERVED);
    assert.deepEqual([wiki.primary, wiki.order], [false, 0]);
  });

  it('puts the primary wiki first, then sorts by order, then by id', () => {
    const wikis = parseWikis(
      JSON.stringify([
        entry('c-late', { order: 5 }),
        entry('b-early', { order: 1 }),
        entry('primary-one', { primary: true, order: 9 }),
        entry('a-early', { order: 1 }),
      ]),
      RESERVED,
    );
    assert.deepEqual(wikis.map((w) => w.id), ['primary-one', 'a-early', 'b-early', 'c-late']);
  });

  it('sorts negative orders and ids as the chart does', () => {
    const wikis = parseWikis(
      JSON.stringify([entry('ten', { order: 10 }), entry('two', { order: 2 }), entry('neg', { order: -3 })]),
      RESERVED,
    );
    assert.deepEqual(wikis.map((w) => w.id), ['neg', 'two', 'ten']);
  });

  const invalid: [string, string, RegExp][] = [
    ['not JSON', '{nope', /not valid JSON/],
    ['an object, not a list', '{"id":"a"}', /must be a list/],
    ['an entry that is not an object', '["a"]', /\[0\]: must be an object/],
    ['a missing id', JSON.stringify([{ label: 'A', url: 'http://a.example/' }]), /\[0\]: id is required/],
    ['an id with capitals', JSON.stringify([entry('Bad')]), /\[0\]: invalid id "Bad"/],
    ['an id that Luigi reads as a dynamic segment', JSON.stringify([entry(':x')]), /invalid id ":x"/],
    ['an id with a slash', JSON.stringify([entry('a/b')]), /invalid id "a\/b"/],
    ['an id of 43 characters', JSON.stringify([entry('a'.repeat(43))]), /invalid id/],
    ['a duplicate id', JSON.stringify([entry('a'), entry('b'), entry('a')]), /\[2\]: duplicate id "a"/],
    ['an id that is a portal page', JSON.stringify([entry('plugins')]), /id "plugins" is already a portal page/],
    ['a missing label', JSON.stringify([{ id: 'a', url: 'http://a.example/' }]), /\[0\]: label is required/],
    ['an empty label', JSON.stringify([entry('a', { label: ' ' })]), /label is required/],
    ['a missing url', JSON.stringify([{ id: 'a', label: 'A' }]), /\[0\]: url is required/],
    ['a url that is not a URL', JSON.stringify([entry('a', { url: 'nope' })]), /url must be an http\(s\) URL/],
    ['a url with another scheme', JSON.stringify([entry('a', { url: 'javascript:alert(1)' })]), /url must be an http\(s\) URL/],
    ['a url with credentials', JSON.stringify([entry('a', { url: 'http://u:p@a.example/' })]), /url must be an http\(s\) URL/],
    ['a url with a query', JSON.stringify([entry('a', { url: 'http://a.example/?x=1' })]), /url must be an http\(s\) URL/],
    ['a url with a fragment', JSON.stringify([entry('a', { url: 'http://a.example/#x' })]), /url must be an http\(s\) URL/],
    ['primary that is not a boolean', JSON.stringify([entry('a', { primary: 'yes' })]), /primary must be true or false/],
    ['an order that is not a number', JSON.stringify([entry('a', { order: '2' })]), /order must be a whole number/],
    ['an order that is not whole', JSON.stringify([entry('a', { order: 1.5 })]), /order must be a whole number/],
  ];
  for (const [name, content, message] of invalid) {
    it(`rejects ${name}`, () => {
      assert.throws(() => parseWikis(content, RESERVED), message);
    });
  }
});

describe('wikiNodes', () => {
  const wikis: Wiki[] = [
    { id: 'handbook', label: 'Platform Handbook', url: 'http://handbook.wiki.localhost:3002/', primary: true, order: 0 },
    { id: 'runbooks', label: 'Runbooks', url: 'https://runbooks.wiki.example.com/', primary: false, order: 10 },
  ];

  it('maps a wiki to a Luigi virtual tree node in the Docs group', () => {
    const [node] = wikiNodes(wikis, DOCS, 9);
    assert.deepEqual(node, {
      pathSegment: 'handbook',
      icon: 'assets/icons/wiki.svg',
      label: 'Platform Handbook',
      order: 9,
      category: DOCS,
      entityType: 'naira',
      hideFromNav: false,
      url: 'http://handbook.wiki.localhost:3002',
      virtualTree: true,
      loadingIndicator: { enabled: false },
    });
  });

  // Luigi builds the page URL as viewUrl + "/" + ":virtualSegment_1", so a trailing slash gives "//guide".
  it('drops the trailing slash of the url', () => {
    assert.deepEqual(wikiNodes(wikis, DOCS, 9).map((n) => n.url), [
      'http://handbook.wiki.localhost:3002',
      'https://runbooks.wiki.example.com',
    ]);
  });

  it('numbers the nodes from firstOrder in list order, so the first wiki comes first', () => {
    assert.deepEqual(wikiNodes(wikis, DOCS, 9).map((n) => [n.pathSegment, n.order]), [
      ['handbook', 9],
      ['runbooks', 10],
    ]);
  });

  it('gives an empty list no nodes', () => {
    assert.deepEqual(wikiNodes([], DOCS, 9), []);
  });
});

describe('loadWikiNodes', () => {
  it('reads the file and maps the wikis, primary first', () => {
    const { nodes, errors } = load(file([entry('later', { order: 3 }), entry('first', { primary: true, order: 8 })]));
    assert.deepEqual(nodes.map((n) => [n.pathSegment, n.order]), [
      ['first', 9],
      ['later', 10],
    ]);
    assert.deepEqual(errors, []);
  });

  it('returns no nodes and stays silent for an empty list', () => {
    assert.deepEqual(load(file([])), { nodes: [], errors: [] });
  });

  it('returns no nodes and stays silent when no file is configured', () => {
    assert.deepEqual(load(undefined), { nodes: [], errors: [] });
    assert.deepEqual(load(''), { nodes: [], errors: [] });
  });

  it('logs one error and returns no nodes for a file that does not exist', () => {
    const { nodes, errors } = load(path.join(dir, 'missing.json'));
    assert.deepEqual(nodes, []);
    assert.equal(errors.length, 1);
    assert.match(errors[0], /missing\.json/);
  });

  it('logs one error and returns no nodes for a file that is not JSON', () => {
    const { nodes, errors } = load(file('{nope'));
    assert.deepEqual(nodes, []);
    assert.equal(errors.length, 1);
    assert.match(errors[0], /not valid JSON/);
  });

  it('logs one error and returns no nodes when a single entry is invalid', () => {
    const { nodes, errors } = load(file([entry('good'), entry('Bad')]));
    assert.deepEqual(nodes, []);
    assert.equal(errors.length, 1);
    assert.match(errors[0], /\[1\]: invalid id "Bad"/);
  });

  it('names the file in the error', () => {
    const name = file('[]x');
    assert.match(load(name).errors[0], new RegExp(path.basename(name).replace('.', '\\.')));
  });

  it('logs one error for a directory', () => {
    const { nodes, errors } = load(dir);
    assert.deepEqual(nodes, []);
    assert.equal(errors.length, 1);
  });
});
