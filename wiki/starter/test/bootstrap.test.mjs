import assert from 'node:assert/strict';
import { test } from 'node:test';
import { recordAttempt, resolveReturnUrl } from '../docs/public/_naira/assets/bootstrap.js';

const origin = 'https://platform-handbook.wiki.naira.example.com';
const root = `${origin}/`;

test('keeps a same-origin path with its query and fragment', () => {
  assert.equal(
    resolveReturnUrl('/guide/getting-started', origin),
    `${origin}/guide/getting-started`,
  );
  assert.equal(
    resolveReturnUrl('/search?q=a%20b#result', origin),
    `${origin}/search?q=a%20b#result`,
  );
});

test('falls back to the site root when there is nothing usable', () => {
  for (const value of [null, undefined, '', 'guide/getting-started', 42, {}]) {
    assert.equal(resolveReturnUrl(value, origin), root, String(value));
  }
});

test('never leaves the wiki origin', () => {
  const hostile = [
    '//evil.example',
    '///evil.example',
    '/\\evil.example',
    '\\\\evil.example',
    'https://evil.example/',
    'http://evil.example',
    'javascript:alert(1)',
    'data:text/html,x',
    ' //evil.example',
    '/\t/evil.example',
    '/\n/evil.example',
    '/\r/evil.example',
    '/\u0000/evil.example',
  ];
  for (const value of hostile) {
    const result = resolveReturnUrl(value, origin);
    assert.equal(new URL(result).origin, origin, JSON.stringify(value));
    assert.equal(result, root, JSON.stringify(value));
  }
});

test('stays on the origin when dot segments collapse to a double slash', () => {
  const result = resolveReturnUrl('/.//evil.example', origin);
  assert.equal(new URL(result).origin, origin);
});

test('never returns to the bootstrap page, which would loop', () => {
  for (const value of [
    '/_naira',
    '/_naira/',
    '/_naira/bootstrap.html',
    '/_naira/bootstrap.html?return=/guide',
    '/guide/../_naira/bootstrap.html',
  ]) {
    assert.equal(resolveReturnUrl(value, origin), root, value);
  }
});

test('allows three sign-in attempts in 30 seconds and stops the fourth', () => {
  let attempts = [];
  for (const now of [0, 1000, 2000]) {
    const result = recordAttempt(attempts, now);
    assert.equal(result.exhausted, false, `attempt at ${now}`);
    attempts = result.attempts;
  }
  assert.equal(recordAttempt(attempts, 3000).exhausted, true);
});

test('forgets sign-in attempts older than 30 seconds', () => {
  const result = recordAttempt([0, 1000, 2000], 40000);
  assert.equal(result.exhausted, false);
  assert.deepEqual(result.attempts, [40000]);
});

test('ignores stored attempts that are not timestamps', () => {
  for (const stored of ['x', null, undefined, {}, [null, 'a', NaN]]) {
    const result = recordAttempt(stored, 5);
    assert.equal(result.exhausted, false, String(stored));
    assert.deepEqual(result.attempts, [5]);
  }
});
