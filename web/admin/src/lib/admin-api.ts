/** Calls to the admin API through this site's /api proxy, with refusals turned into plain messages. */
import type { components } from '@ttac/shared/api';

export type AdminMe = components['schemas']['AdminMe'];
export type AdminTournament = components['schemas']['AdminTournament'];
export type TournamentSettings = components['schemas']['TournamentSettings'];
export type SeasonNote = components['schemas']['SeasonNote'];
export type AdminPerson = components['schemas']['AdminPerson'];
export type AdminRun = components['schemas']['AdminRun'];
export type RunChanges = components['schemas']['RunChanges'];
export type StartedRun = components['schemas']['StartedRun'];

/** AdminResult is an answer's data, or why the request was refused. */
export type AdminResult<T> = { ok: true; data: T } | { ok: false; status: number; message: string };

type Fetch = (input: string, init: RequestInit) => Promise<Response>;

function refusal(status: number, error: string | undefined): string {
  if (status === 401) return 'Your sign-in has ended. Reload the page to sign in again.';
  if ([400, 403, 404, 409, 503].includes(status)) return `Refused: ${error ?? 'not allowed'}.`;
  return `The API failed (${status}). Try again, or tell an admin.`;
}

/** adminRequest calls an admin path on this site, sending json as the body when given, and returns its result. */
export async function adminRequest<T>(
  path: string,
  { method = 'GET', json }: { method?: string; json?: unknown },
  fetcher: Fetch = fetch,
): Promise<AdminResult<T>> {
  const init: RequestInit = { method, credentials: 'same-origin' };
  if (json !== undefined) {
    init.headers = { 'Content-Type': 'application/json' };
    init.body = JSON.stringify(json);
  }
  let res: Response;
  try {
    res = await fetcher(path, init);
  } catch {
    return { ok: false, status: 0, message: 'The admin site could not be reached. Check your connection.' };
  }
  if (res.status === 204) return { ok: true, data: null as T };
  let body: unknown = null;
  try {
    body = await res.json();
  } catch {
    body = null;
  }
  if (res.ok) return { ok: true, data: body as T };
  const error = typeof body === 'object' && body !== null && 'error' in body ? String(body.error) : undefined;
  return { ok: false, status: res.status, message: refusal(res.status, error) };
}
