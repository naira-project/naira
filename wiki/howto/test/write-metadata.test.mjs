import assert from 'node:assert/strict';
import { test } from 'node:test';
import { buildMetadata, CONTRACT } from '../scripts/write-metadata.mjs';

const now = new Date('2026-11-07T10:00:00Z');
const pkg = {
  name: 'platform-handbook',
  version: '1.4.0',
  repository: { type: 'git', url: 'git+https://github.com/acme/platform-handbook.git' },
  bugs: { url: 'https://github.com/acme/platform-handbook/issues' },
  naira: { title: 'Platform Handbook', owner: 'team-platform' },
};

const build = ({ env = {}, ...rest } = {}) =>
  buildMetadata({ env, pkg, generatorVersion: '1.6.4', now, ...rest });

test('writes the v1alpha1 contract document', () => {
  const meta = build({ env: { NAIRA_WIKI_ID: 'platform-handbook', GITHUB_SHA: 'a1b2c3d' } });
  assert.equal(CONTRACT, 'naira.wiki/v1alpha1');
  assert.deepEqual(meta, {
    contract: 'naira.wiki/v1alpha1',
    id: 'platform-handbook',
    title: 'Platform Handbook',
    owner: 'team-platform',
    source: { repository: 'https://github.com/acme/platform-handbook', commit: 'a1b2c3d' },
    support: 'https://github.com/acme/platform-handbook/issues',
    version: '1.4.0',
    builtAt: '2026-11-07T10:00:00Z',
    generator: { name: 'vitepress', version: '1.6.4' },
  });
});

test('takes the id from NAIRA_WIKI_ID, then from the package name', () => {
  assert.equal(build({ env: { NAIRA_WIKI_ID: 'other-wiki' } }).id, 'other-wiki');
  assert.equal(build({ env: { NAIRA_WIKI_ID: '' } }).id, 'platform-handbook');
  assert.equal(build().id, 'platform-handbook');
});

test('accepts ids that are valid DNS labels up to 42 characters', () => {
  for (const id of ['a', 'a1', 'a-b', '0-9', 'a'.repeat(42)]) {
    assert.equal(build({ env: { NAIRA_WIKI_ID: id } }).id, id);
  }
});

test('rejects ids that are not usable as a DNS label', () => {
  const invalid = ['Platform', '-a', 'a-', 'a_b', 'a.b', 'a b', 'a'.repeat(43), '../x'];
  for (const id of invalid) {
    assert.throws(() => build({ env: { NAIRA_WIKI_ID: id } }), /invalid wiki id/i, id);
  }
});

test('omits what is not known instead of writing empty values', () => {
  const meta = build({
    pkg: { name: 'bare', version: '0.0.1' },
  });
  assert.deepEqual(meta, {
    contract: 'naira.wiki/v1alpha1',
    id: 'bare',
    title: 'bare',
    version: '0.0.1',
    builtAt: '2026-11-07T10:00:00Z',
    generator: { name: 'vitepress', version: '1.6.4' },
  });
  assert.deepEqual(JSON.parse(JSON.stringify(meta)), meta);
});

test('keeps the repository but writes no commit when GITHUB_SHA is unset', () => {
  assert.deepEqual(build().source, { repository: 'https://github.com/acme/platform-handbook' });
});

test('normalises the package repository field', () => {
  const forms = [
    'git+https://github.com/acme/wiki.git',
    'https://github.com/acme/wiki.git',
    'https://github.com/acme/wiki',
  ];
  for (const url of forms) {
    for (const repository of [url, { type: 'git', url }]) {
      const meta = build({ pkg: { ...pkg, repository } });
      assert.equal(
        meta.source.repository,
        'https://github.com/acme/wiki',
        JSON.stringify(repository),
      );
    }
  }
});

test('reads support from bugs as a string or an object', () => {
  const url = 'https://example.com/support';
  assert.equal(build({ pkg: { ...pkg, bugs: url } }).support, url);
  assert.equal(build({ pkg: { ...pkg, bugs: { url } } }).support, url);
});
