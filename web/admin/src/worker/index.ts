/** The admin site's Worker: static pages, and /api/* proxied to the API's admin routes for a verified Access session. */
import { adminApiPath, apiOriginFor, keyCache, verifyAccessToken, type Jwk } from './proxy';

interface Env {
  ASSETS: { fetch(request: Request): Promise<Response> };
  ACCESS_TEAM?: string;
  ACCESS_AUD?: string;
  ADMIN_HOST?: string;
  API_ORIGIN?: string;
}

const KEYS_FOR_MS = 5 * 60 * 1000;
let teamKeys: ((now: number) => Promise<Jwk[]>) | null = null;

async function fetchKeys(team: string): Promise<Jwk[]> {
  const res = await fetch(`${team}/cdn-cgi/access/certs`);
  if (!res.ok) throw new Error(`certs: ${res.status}`);
  const body = (await res.json()) as { keys?: Jwk[] };
  return body.keys ?? [];
}

function refuse(status: number, error: string): Response {
  return Response.json({ error }, { status, headers: { 'Cache-Control': 'no-store' } });
}

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    const url = new URL(request.url);
    if (!url.pathname.startsWith('/api/')) return env.ASSETS.fetch(request);
    const api = apiOriginFor(url.hostname, env.ADMIN_HOST, env.API_ORIGIN);
    if (api === null || !env.ACCESS_TEAM || !env.ACCESS_AUD) return refuse(503, 'admin is not configured');
    const path = adminApiPath(url);
    if (path === null) return refuse(404, 'no such admin route');
    const token = request.headers.get('Cf-Access-Jwt-Assertion') ?? '';
    const team = env.ACCESS_TEAM;
    const keys = (teamKeys ??= keyCache(() => fetchKeys(team), KEYS_FOR_MS));
    if (!(await verifyAccessToken(token, team, env.ACCESS_AUD, Date.now() / 1000, () => keys(Date.now())))) {
      return refuse(401, 'sign in again');
    }
    const headers = new Headers({ 'Cf-Access-Jwt-Assertion': token, Accept: 'application/json' });
    const type = request.headers.get('Content-Type');
    if (type) headers.set('Content-Type', type);
    const origin = request.headers.get('Origin');
    if (origin) headers.set('Origin', origin);
    const body = request.method === 'GET' || request.method === 'HEAD' ? undefined : await request.arrayBuffer();
    const res = await fetch(`${api}${path}`, { method: request.method, headers, body });
    const out = new Headers({ 'Content-Type': res.headers.get('Content-Type') ?? 'application/json', 'Cache-Control': 'no-store' });
    for (const name of ['Content-Disposition', 'X-Request-Id']) {
      const value = res.headers.get(name);
      if (value) out.set(name, value);
    }
    return new Response(res.body, { status: res.status, headers: out });
  },
};
