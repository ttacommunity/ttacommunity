/** Calls to the admin API through this site's /api proxy, with refusals turned into plain messages. */
import type { components } from '@ttac/shared/api';
import { errorFields, type ErrorFields } from '@ttac/shared/ratings';
import type { Load } from '@ttac/shared/load';

export type AdminMe = components['schemas']['AdminMe'];
export type AdminTournament = components['schemas']['AdminTournament'];
export type AdminTournamentRow = components['schemas']['AdminTournamentRow'];
/** TournamentBrief is what a list row and a whole tournament share. */
export type TournamentBrief = Omit<AdminTournamentRow, 'cge_about'>;
export type TournamentSettings = components['schemas']['TournamentSettings'];
export type AdminPerson = components['schemas']['AdminPerson'];
export type AdminAuditPage = components['schemas']['AdminAuditPage'];
export type AdminRun = components['schemas']['AdminRun'];
export type RunChanges = components['schemas']['RunChanges'];
export type StartedRun = components['schemas']['StartedRun'];
export type AdminBlitzEditionRow = components['schemas']['AdminBlitzEditionRow'];
export type BlitzDiscordText = components['schemas']['BlitzDiscordText'];
export type AdminBlitzEdition = components['schemas']['AdminBlitzEdition'];
export type BlitzEntrantIds = components['schemas']['BlitzEntrantIds'];
export type AdminMember = components['schemas']['AdminMember'];
export type MemberAdded = components['schemas']['MemberAdded'];
export type MemberChange = components['schemas']['MemberChange'];
export type AdminAccount = components['schemas']['AdminAccount'];
export type LinkApproved = components['schemas']['LinkApproved'];

/** AdminResult is an answer's data, or why the request was refused. */
export type AdminResult<T> = { ok: true; data: T } | { ok: false; status: number; message: string };

/** firstError returns the message of the first load that failed, or null when none did: a page shows one. */
export function firstError(...loads: Load<unknown>[]): string | null {
  for (const l of loads) if (l.kind === 'error') return l.message;
  return null;
}

type Fetch = (input: string, init: RequestInit) => Promise<Response>;

function refusal(status: number, { error, requestId }: ErrorFields): string {
  if (status === 401) return 'Your sign-in has ended. Reload the page to sign in again.';
  if (status === 502) return 'The API could not be reached. Try again in a minute.';
  if ([400, 403, 404, 409, 413, 503].includes(status)) return `Refused: ${error ?? 'not allowed'}.`;
  const ref = requestId === undefined ? '' : ` Quote ref ${requestId}.`;
  return `The API failed (${status}). Try again, or tell an admin.${ref}`;
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
  return { ok: false, status: res.status, message: refusal(res.status, errorFields(body)) };
}

/** adminDownload fetches a file from an admin path on this site, or why it was refused. */
export async function adminDownload(path: string, fetcher: Fetch = fetch): Promise<AdminResult<Blob>> {
  let res: Response;
  try {
    res = await fetcher(path, { method: 'GET', credentials: 'same-origin' });
  } catch {
    return { ok: false, status: 0, message: 'The admin site could not be reached. Check your connection.' };
  }
  if (res.ok) {
    try {
      return { ok: true, data: await res.blob() };
    } catch {
      return { ok: false, status: 0, message: 'The download broke off part way. Try again.' };
    }
  }
  let body: unknown = null;
  try {
    body = await res.json();
  } catch {
    body = null;
  }
  return { ok: false, status: res.status, message: refusal(res.status, errorFields(body)) };
}

/** exportFileName names a data download by the local day it was taken. */
export function exportFileName(at: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0');
  return `ttacommunity-data-${at.getFullYear()}-${pad(at.getMonth() + 1)}-${pad(at.getDate())}.zip`;
}
