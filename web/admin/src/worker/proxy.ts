/** The admin Worker's API proxy: verifies the Access token, then forwards /api/* to the API's /v1/admin/*. */

/** Jwk is one of the team's signing keys as its certs endpoint lists them. */
export interface Jwk extends JsonWebKey {
  kid?: string;
}

/**
 * apiOriginFor returns `apiOrigin` when the admin site was reached on `adminHost`, its configured host,
 * or null for any other host, either setting missing, or an API not on https.
 */
export function apiOriginFor(host: string, adminHost: string | undefined, apiOrigin: string | undefined): string | null {
  if (!adminHost || !apiOrigin || host !== adminHost) return null;
  // The Access token is forwarded to it, so never over plain http.
  return apiOrigin.startsWith('https://') ? apiOrigin : null;
}

const PLAIN_PATH = /^\/api\/[A-Za-z0-9_-]+(\/[A-Za-z0-9_.-]+)*$/;

/** adminApiPath returns the API path for an /api request (with its query), or null for anything else. */
export function adminApiPath(url: URL): string | null {
  if (!PLAIN_PATH.test(url.pathname) || url.pathname.includes('..')) return null;
  return `/v1/admin/${url.pathname.slice('/api/'.length)}${url.search}`;
}

function bytes(part: string): Uint8Array<ArrayBuffer> {
  const raw = atob(part.replace(/-/g, '+').replace(/_/g, '/'));
  const out = new Uint8Array(new ArrayBuffer(raw.length));
  for (let i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i);
  return out;
}

function decode(part: string): unknown {
  return JSON.parse(new TextDecoder().decode(bytes(part)));
}

interface Claims {
  aud?: string | string[];
  iss?: string;
  exp?: number;
  nbf?: number;
  type?: string;
  custom?: { id?: unknown };
}

/**
 * verifyAccessToken reports whether token is an RS256 Access token signed by one of keys(), issued by team, for aud,
 * current at now (seconds), for an app session, and carrying a Discord id. Anything unexpected is a refusal.
 */
export async function verifyAccessToken(
  token: string,
  team: string,
  aud: string,
  now: number,
  keys: () => Promise<Jwk[]>,
): Promise<boolean> {
  if (!team || !aud) return false;
  const parts = token.split('.');
  if (parts.length !== 3) return false;
  try {
    const [h, p, s] = parts as [string, string, string];
    const header = decode(h) as { alg?: string; kid?: string };
    const claims = decode(p) as Claims;
    if (header.alg !== 'RS256' || !header.kid) return false;
    const jwk = (await keys()).find((k) => k.kid === header.kid && k.kty === 'RSA');
    if (!jwk) return false;
    const key = await crypto.subtle.importKey(
      'jwk',
      { kty: 'RSA', n: jwk.n, e: jwk.e, alg: 'RS256', ext: true },
      { name: 'RSASSA-PKCS1-v1_5', hash: 'SHA-256' },
      false,
      ['verify'],
    );
    const signed = await crypto.subtle.verify('RSASSA-PKCS1-v1_5', key, bytes(s), new TextEncoder().encode(`${h}.${p}`));
    const auds = Array.isArray(claims.aud) ? claims.aud : [claims.aud];
    return (
      signed &&
      claims.iss === team &&
      auds.includes(aud) &&
      typeof claims.exp === 'number' &&
      claims.exp > now &&
      (claims.nbf === undefined || claims.nbf <= now) &&
      claims.type === 'app' &&
      typeof claims.custom?.id === 'string' &&
      /^[0-9]{5,25}$/.test(claims.custom.id)
    );
  } catch {
    return false;
  }
}

/** keyCache returns a function giving the keys load fetches, fetched again only once they are forMs old (now in ms). */
export function keyCache(load: () => Promise<Jwk[]>, forMs: number): (now: number) => Promise<Jwk[]> {
  let cached: { keys: Jwk[]; at: number } | null = null;
  return async (now) => {
    if (cached !== null && now - cached.at < forMs) return cached.keys;
    cached = { keys: await load(), at: now };
    return cached.keys;
  };
}
