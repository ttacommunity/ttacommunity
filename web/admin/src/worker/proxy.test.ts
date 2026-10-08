import { describe, expect, it } from 'vitest';
import { adminApiPath, apiOriginFor, keyCache, verifyAccessToken, type Jwk } from './proxy';

const TEAM = 'https://team.cloudflareaccess.com';
const AUD = 'aud-of-the-admin-app';
const NOW = Date.UTC(2026, 9, 3, 12) / 1000;

function b64url(b: ArrayBuffer | string): string {
  const raw = typeof b === 'string' ? new TextEncoder().encode(b) : new Uint8Array(b);
  return btoa(String.fromCharCode(...raw)).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

async function keyPair() {
  const pair = await crypto.subtle.generateKey(
    { name: 'RSASSA-PKCS1-v1_5', modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: 'SHA-256' },
    true,
    ['sign', 'verify'],
  );
  const jwk = (await crypto.subtle.exportKey('jwk', pair.publicKey)) as Jwk;
  return { pair, jwk: { ...jwk, kid: 'key-1' } };
}

async function token(pair: CryptoKeyPair, claims: Record<string, unknown>, header: Record<string, unknown> = {}) {
  const h = b64url(JSON.stringify({ alg: 'RS256', kid: 'key-1', typ: 'JWT', ...header }));
  const p = b64url(JSON.stringify(claims));
  const sig = await crypto.subtle.sign('RSASSA-PKCS1-v1_5', pair.privateKey, new TextEncoder().encode(`${h}.${p}`));
  return `${h}.${p}.${b64url(sig)}`;
}

const good = {
  aud: [AUD],
  iss: TEAM,
  exp: NOW + 3600,
  nbf: NOW - 60,
  iat: NOW - 60,
  type: 'app',
  custom: { id: '100000000000000001' },
};

describe('adminApiPath', () => {
  it("maps the admin site's /api paths onto the API's admin routes, with the query", () => {
    expect(adminApiPath(new URL('https://a.example/api/me'))).toBe('/v1/admin/me');
    expect(adminApiPath(new URL('https://a.example/api/tournaments/worlds?x=1'))).toBe('/v1/admin/tournaments/worlds?x=1');
  });

  it('refuses anything that is not a plain admin path', () => {
    for (const p of ['/api', '/api/', '/me', '/api/%2e%2e/runs', '/api/a%2Fb', '/api//me']) {
      expect(adminApiPath(new URL(`https://a.example${p}`)), p).toBeNull();
    }
  });
});

describe('verifyAccessToken', () => {
  it('accepts a valid token from the team for the admin app', async () => {
    const { pair, jwk } = await keyPair();
    expect(await verifyAccessToken(await token(pair, good), TEAM, AUD, NOW, async () => [jwk])).toBe(true);
  });

  it('refuses a token that is wrong in any way', async () => {
    const { pair, jwk } = await keyPair();
    const other = await keyPair();
    const cases: [string, string][] = [
      ['empty', ''],
      ['not a token', 'a.b.c'],
      ['another audience', await token(pair, { ...good, aud: ['another-app'] })],
      ['another issuer', await token(pair, { ...good, iss: 'https://evil.example' })],
      ['expired', await token(pair, { ...good, exp: NOW - 1 })],
      ['not yet valid', await token(pair, { ...good, nbf: NOW + 60 })],
      ['signed by another key', await token(other.pair, good)],
      ['another algorithm', await token(pair, good, { alg: 'HS256' })],
      ['unknown key id', await token(pair, good, { kid: 'key-2' })],
      ['a service token', await token(pair, { ...good, type: 'org' })],
      ['no Discord id', await token(pair, { ...good, custom: {} })],
    ];
    for (const [name, t] of cases) {
      expect(await verifyAccessToken(t, TEAM, AUD, NOW, async () => [jwk]), name).toBe(false);
    }
  });

  it('refuses everything when the audience or team is not set', async () => {
    const { pair, jwk } = await keyPair();
    const t = await token(pair, good);
    expect(await verifyAccessToken(t, TEAM, '', NOW, async () => [jwk])).toBe(false);
    expect(await verifyAccessToken(t, '', AUD, NOW, async () => [jwk])).toBe(false);
  });
});

describe('apiOriginFor', () => {
  it("sends the admin site to the main API and its dev preview to the dev API", () => {
    expect(apiOriginFor('ttac-admin.unstableunicorn.dev')).toBe('https://ttac-api.unstableunicorn.dev');
    expect(apiOriginFor('dev.ttac-admin.unstableunicorn.dev')).toBe('https://ttac-api-dev.unstableunicorn.dev');
  });

  it('gives no API to any other host, such as a deployment preview', () => {
    for (const host of ['3f2a9c1b.ttac-admin.unstableunicorn.dev', 'evil.example', 'ttac-admin.unstableunicorn.dev.evil.example', '']) {
      expect(apiOriginFor(host), host).toBeNull();
    }
  });
});

describe('keyCache', () => {
  it.each([
    ['not again within the time kept', 1_000, 1_000 + 299_999, 1],
    ['again once the time kept is over', 1_000, 1_000 + 300_000, 2],
  ])('fetches the keys once, and %s', async (_name, first, second, want) => {
    let loads = 0;
    const keys = keyCache(async () => {
      loads++;
      return [{ kid: `k${loads}` }];
    }, 300_000);
    await keys(first);
    await keys(second);
    expect(loads).toBe(want);
  });
});
