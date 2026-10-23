import assert from 'node:assert/strict';
import { once } from 'node:events';
import { createServer, request } from 'node:http';
import { test } from 'node:test';
import { NestFactory } from '@nestjs/core';
import { CookiesService } from '@openmfp/portal-server-lib';
import { AppModule } from '../dist/app.module.js';
import { NairaCookiesService } from '../dist/cookies.service.js';

function configureEnv(t, values) {
  for (const [key, value] of Object.entries(values)) {
    const previous = process.env[key];
    if (value === undefined) delete process.env[key];
    else process.env[key] = value;
    t.after(() => {
      if (previous === undefined) delete process.env[key];
      else process.env[key] = previous;
    });
  }
}

const cases = [
  ['secure by default', undefined, 'localhost', 'http', true],
  ['explicitly disabled exception', 'false', 'localhost', 'http', true],
  ['invalid option fails closed', '1', 'localhost', 'http', true],
  ['local HTTP opt-in', 'true', 'localhost', 'http', false],
  ['local HTTPS stays secure', 'true', 'localhost', 'https', true],
  ['remote HTTP stays secure', 'true', 'portal.example.com', 'http', true],
  ['remote HTTPS stays secure', 'true', 'portal.example.com', 'https', true],
  ['lookalike hostname stays secure', 'true', 'localhost.example.com', 'http', true],
];

for (const [name, flag, hostname, protocol, secure] of cases) {
  test(name, (t) => {
    configureEnv(t, { ALLOW_INSECURE_LOCALHOST_COOKIES: flag });
    const service = new NairaCookiesService();
    const req = { hostname, protocol, cookies: { openmfp_auth_cookie: 'refresh-token' } };
    const expected = { httpOnly: true, secure, path: '/', sameSite: 'strict' };
    let setOptions;
    let clearOptions;
    service.setAuthCookie(req, {
      cookie(name, value, options) {
        assert.equal(name, 'openmfp_auth_cookie');
        assert.equal(value, 'refresh-token');
        setOptions = options;
      },
    }, { refresh_token: 'refresh-token' });
    service.removeAuthCookie(req, {
      clearCookie(name, options) {
        assert.equal(name, 'openmfp_auth_cookie');
        clearOptions = options;
      },
    });
    assert.deepEqual(setOptions, expected);
    assert.deepEqual(clearOptions, expected);
    assert.equal(service.getAuthCookie(req), 'refresh-token');
  });
}

function send(port, path, { method = 'GET', cookie } = {}) {
  return new Promise((resolve, reject) => {
    const req = request({
      hostname: '127.0.0.1', port, path, method,
      headers: { Host: `localhost:${port}`, ...(cookie ? { Cookie: cookie } : {}) },
    }, (res) => {
      let body = '';
      res.setEncoding('utf8');
      res.on('data', (chunk) => { body += chunk; });
      res.on('end', () => resolve({ status: res.statusCode, headers: res.headers, body }));
    });
    req.on('error', reject);
    req.end();
  });
}

test('HTTP callback and repeated refresh retain the session through the real portal module', async (t) => {
  const grants = [];
  const tokenServer = createServer(async (req, res) => {
    let body = '';
    for await (const chunk of req) body += chunk;
    grants.push(new URLSearchParams(body));
    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({
      access_token: 'access-token', id_token: 'id-token', expires_in: 300,
      refresh_token: `refresh-${grants.length}`, refresh_expires_in: 1800,
      token_type: 'Bearer',
    }));
  });
  tokenServer.listen(0, '127.0.0.1');
  await once(tokenServer, 'listening');
  t.after(() => new Promise((resolve) => tokenServer.close(resolve)));
  configureEnv(t, {
    ALLOW_INSECURE_LOCALHOST_COOKIES: 'true',
    IDP_NAMES: 'keycloak', BASE_DOMAINS_KEYCLOAK: 'localhost',
    OIDC_CLIENT_ID_KEYCLOAK: 'test-client', OIDC_CLIENT_SECRET_KEYCLOAK: 'test-secret',
    DISCOVERY_ENDPOINT_KEYCLOAK: undefined,
    AUTH_SERVER_URL_KEYCLOAK: `http://127.0.0.1:${tokenServer.address().port}/auth`,
    TOKEN_URL_KEYCLOAK: `http://127.0.0.1:${tokenServer.address().port}/token`,
  });
  const app = await NestFactory.create(AppModule, { logger: false });
  t.after(() => app.close());
  await app.listen(0, '127.0.0.1');
  assert.ok(app.get(CookiesService) instanceof NairaCookiesService);
  const port = app.getHttpServer().address().port;
  const home = `http://localhost:${port}/`;
  const state = Buffer.from(`${home}_luigiNonce=test`).toString('base64');
  let response = await send(port, `/callback?${new URLSearchParams({ code: 'test-code', state })}`);
  assert.equal(response.status, 302);
  assert.equal(response.headers.location, home);
  assert.equal(grants[0].get('grant_type'), 'authorization_code');

  for (let i = 1; i <= 2; i++) {
    const cookie = response.headers['set-cookie'].find((value) => value.startsWith('openmfp_auth_cookie='));
    assert.ok(cookie);
    assert.doesNotMatch(cookie, /; Secure(?:;|$)/i);
    assert.match(cookie, /; HttpOnly/i);
    assert.match(cookie, /; SameSite=Strict/i);
    response = await send(port, '/rest/auth/refresh', { method: 'POST', cookie: cookie.split(';')[0] });
    assert.equal(response.status, 201);
    const tokens = JSON.parse(response.body);
    assert.equal(tokens.id_token, 'id-token');
    assert.equal(tokens.refresh_token, undefined);
    assert.equal(tokens.refresh_expires_in, undefined);
    assert.equal(grants[i].get('grant_type'), 'refresh_token');
    assert.equal(grants[i].get('refresh_token'), `refresh-${i}`);
  }
});