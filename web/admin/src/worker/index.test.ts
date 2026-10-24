import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import worker from './index';
import type { Jwk } from './proxy';

const TEAM = 'https://team.cloudflareaccess.com';
const AUD = 'aud-of-the-admin-app';
const SITE = 'https://ttac-admin.example';
const API = 'https://api.example';

function b64url(b: ArrayBuffer | string): string {
  const raw = typeof b === 'string' ? new TextEncoder().encode(b) : new Uint8Array(b);
  return btoa(String.fromCharCode(...raw)).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

let pair: CryptoKeyPair;
let jwk: Jwk;
let good: string;

async function sign(claims: Record<string, unknown>, key: CryptoKey): Promise<string> {
  const h = b64url(JSON.stringify({ alg: 'RS256', kid: 'key-1', typ: 'JWT' }));
  const p = b64url(JSON.stringify(claims));
  const sig = await crypto.subtle.sign('RSASSA-PKCS1-v1_5', key, new TextEncoder().encode(`${h}.${p}`));
  return `${h}.${p}.${b64url(sig)}`;
}

beforeAll(async () => {
  const params = { name: 'RSASSA-PKCS1-v1_5', modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: 'SHA-256' };
  pair = (await crypto.subtle.generateKey(params, true, ['sign', 'verify'])) as CryptoKeyPair;
  jwk = { ...((await crypto.subtle.exportKey('jwk', pair.publicKey)) as Jwk), kid: 'key-1' };
  const now = Math.floor(Date.now() / 1000);
  good = await sign({ aud: [AUD], iss: TEAM, exp: now + 3600, nbf: now - 60, iat: now - 60, type: 'app', custom: { id: '111111111111111111' } }, pair.privateKey);
});

afterEach(() => vi.unstubAllGlobals());

/** network stubs the certificates and the API (unreachable when apiDown), recording every request the Worker makes. */
function network({ apiDown = false } = {}) {
  const calls: { url: string; init?: RequestInit }[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      calls.push({ url, init });
      if (url === `${TEAM}/cdn-cgi/access/certs`) return Response.json({ keys: [jwk] });
      if (apiDown) throw new TypeError('Network connection lost.');
      if (url.endsWith('/v1/admin/export')) {
        return new Response('PK zip', {
          headers: { 'Content-Type': 'application/zip', 'Content-Disposition': 'attachment; filename="ttacommunity-data.zip"' },
        });
      }
      return Response.json({ ok: true }, { status: 200, headers: { 'X-Request-Id': '0123456789abcdef' } });
    }),
  );
  return calls;
}

function env(over: Record<string, unknown> = {}) {
  const assets = vi.fn(async () => new Response('page'));
  return { env: { ASSETS: { fetch: assets }, ACCESS_TEAM: TEAM, ACCESS_AUD: AUD, ADMIN_HOST: 'ttac-admin.example', API_ORIGIN: API, ...over }, assets };
}

describe('the admin Worker', () => {
  it('serves pages from the static assets without touching the API', async () => {
    const calls = network();
    const { env: e, assets } = env();
    const res = await worker.fetch(new Request(`${SITE}/tournament/?t=worlds`), e);
    expect(await res.text()).toBe('page');
    expect(assets).toHaveBeenCalledOnce();
    expect(calls).toEqual([]);
  });

  it('refuses /api on a host it does not know, or without its Access settings, before any request', async () => {
    const calls = network();
    const unknownHost = await worker.fetch(new Request('https://evil.example/api/me', { headers: { 'Cf-Access-Jwt-Assertion': good } }), env().env);
    const noAud = await worker.fetch(new Request(`${SITE}/api/me`, { headers: { 'Cf-Access-Jwt-Assertion': good } }), env({ ACCESS_AUD: '' }).env);
    const noHost = await worker.fetch(new Request(`${SITE}/api/me`, { headers: { 'Cf-Access-Jwt-Assertion': good } }), env({ ADMIN_HOST: undefined }).env);
    expect([unknownHost.status, noAud.status, noHost.status]).toEqual([503, 503, 503]);
    expect(calls).toEqual([]);
  });

  it('refuses a path that is not a plain admin route', async () => {
    network();
    const res = await worker.fetch(new Request(`${SITE}/api/me%2F..%2Fadmins`, { headers: { 'Cf-Access-Jwt-Assertion': good } }), env().env);
    expect(res.status).toBe(404);
  });

  it('refuses a request without a token, or with one not signed by Access, and never reaches the API', async () => {
    const calls = network();
    const other = (await crypto.subtle.generateKey(
      { name: 'RSASSA-PKCS1-v1_5', modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: 'SHA-256' },
      true,
      ['sign', 'verify'],
    )) as CryptoKeyPair;
    const now = Math.floor(Date.now() / 1000);
    const forged = await sign({ aud: [AUD], iss: TEAM, exp: now + 3600, type: 'app', custom: { id: '111111111111111111' } }, other.privateKey);
    for (const token of ['', forged]) {
      const res = await worker.fetch(new Request(`${SITE}/api/me`, { headers: { 'Cf-Access-Jwt-Assertion': token } }), env().env);
      expect(res.status).toBe(401);
      expect(res.headers.get('Cache-Control')).toBe('no-store');
    }
    expect(calls.filter((c) => c.url.startsWith(API))).toEqual([]);
  });

  it("forwards a signed-in change to the host's API with its method, origin and body, and is never cached", async () => {
    const calls = network();
    const res = await worker.fetch(
      new Request(`${SITE}/api/tournaments/worlds/card`, {
        method: 'PUT',
        headers: { 'Cf-Access-Jwt-Assertion': good, 'Content-Type': 'application/json', Origin: SITE },
        body: '{"icon":"🏆"}',
      }),
      env().env,
    );
    expect(res.status).toBe(200);
    expect(res.headers.get('Cache-Control')).toBe('no-store');
    const sent = calls.find((c) => c.url.startsWith(API));
    expect(sent?.url).toBe(`${API}/v1/admin/tournaments/worlds/card`);
    expect(sent?.init?.method).toBe('PUT');
    const headers = new Headers(sent?.init?.headers);
    expect([headers.get('Origin'), headers.get('Content-Type'), headers.get('Cf-Access-Jwt-Assertion')]).toEqual([SITE, 'application/json', good]);
    expect(new TextDecoder().decode(sent?.init?.body as ArrayBuffer)).toBe('{"icon":"🏆"}');
  });

  it('passes a download through with its type and file name', async () => {
    network();
    const res = await worker.fetch(new Request(`${SITE}/api/export`, { headers: { 'Cf-Access-Jwt-Assertion': good } }), env().env);
    expect(await res.text()).toBe('PK zip');
    expect(res.headers.get('Content-Type')).toBe('application/zip');
    expect(res.headers.get('Content-Disposition')).toBe('attachment; filename="ttacommunity-data.zip"');
  });

  it("passes the API's request id on, so an error can be reported by it", async () => {
    network();
    const res = await worker.fetch(new Request(`${SITE}/api/me`, { headers: { 'Cf-Access-Jwt-Assertion': good } }), env().env);
    expect(res.headers.get('X-Request-Id')).toBe('0123456789abcdef');
  });

  it('answers a JSON 502 when the API cannot be reached', async () => {
    network({ apiDown: true });
    const res = await worker.fetch(new Request(`${SITE}/api/me`, { headers: { 'Cf-Access-Jwt-Assertion': good } }), env().env);
    expect(res.status).toBe(502);
    expect(await res.json()).toEqual({ error: 'the API could not be reached' });
    expect(res.headers.get('Cache-Control')).toBe('no-store');
  });

  it("keeps Access's signing keys for a while rather than fetching them on every request", async () => {
    const calls = network();
    for (let i = 0; i < 3; i++) {
      await worker.fetch(new Request(`${SITE}/api/me`, { headers: { 'Cf-Access-Jwt-Assertion': good } }), env().env);
    }
    expect(calls.filter((c) => c.url.endsWith('/cdn-cgi/access/certs')).length).toBeLessThanOrEqual(1);
  });
});
