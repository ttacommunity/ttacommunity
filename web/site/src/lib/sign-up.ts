/** A player's sign-up for a tournament: what the sign-up box shows, its count and closing lines, and the requests that read, make and withdraw it. Any tournament that takes sign-ups uses it. */
import type { components } from '@ttac/shared/api';
import { formatInstant } from '@ttac/shared/time';
import { AccountApiError, change, failureMessage, type Done } from './account';

export type SignUps = components['schemas']['SignUps'];
export type MySignUp = components['schemas']['MySignUp'];

/** SignUpBox is what the box shows: nothing, a wait while it asks, a sign-in note, the way to sign up (linked, or with a TTA username), or where the player's sign-up stands. */
export type SignUpBox =
  | { kind: 'hidden' }
  | { kind: 'checking' }
  | { kind: 'sign-in' }
  | { kind: 'enter' }
  | { kind: 'give-name' }
  | { kind: 'entered' }
  | { kind: 'pending'; name: string; canWithdraw: boolean };

/** signUpBox decides the box from the tournament's sign-ups and the viewer's (undefined while asking, null for nobody signed in): shown while they take entries, and after only to a player whose sign-up still waits for a link. */
export function signUpBox(s: SignUps, mine: MySignUp | null | undefined): SignUpBox {
  if (mine?.status === 'pending') return { kind: 'pending', name: mine.name ?? '', canWithdraw: s.taking };
  if (!s.taking) return { kind: 'hidden' };
  if (mine === undefined) return { kind: 'checking' };
  if (mine === null) return { kind: 'sign-in' };
  if (mine.status === 'entered') return { kind: 'entered' };
  return mine.linked ? { kind: 'enter' } : { kind: 'give-name' };
}

/** countLine says how many have entered, and how many wait for their account to be linked. */
export function countLine(entered: number, waiting: number): string {
  if (entered === 0 && waiting === 0) return 'No one has signed up yet.';
  const players = entered === 0 ? 'No one entered yet' : `${entered} player${entered === 1 ? '' : 's'} entered`;
  return waiting === 0 ? `${players}.` : `${players}, and ${waiting} waiting for their account to be linked.`;
}

/** closesLine says when sign-ups stop taking entries, in the viewer's zone, named. */
export function closesLine(s: SignUps, zone: string): string {
  return s.closes_at === null ? 'Sign-ups close when round 1 is paired.' : `Sign-ups close on ${formatInstant(s.closes_at, zone)}, or when round 1 is paired.`;
}

function path(slug: string, season: string): string {
  return `/v1/me/sign-ups/${encodeURIComponent(slug)}/${encodeURIComponent(season)}`;
}

/** fetchMySignUp returns where the signed-in player's sign-up stands, or null when nobody is signed in. */
export async function fetchMySignUp(base: string, slug: string, season: string, fetchFn: typeof fetch = fetch): Promise<MySignUp | null> {
  const res = await fetchFn(base.replace(/\/+$/, '') + path(slug, season), { credentials: 'include' });
  if (res.status === 401) return null;
  if (!res.ok) throw new AccountApiError(res.status, await failureMessage(res));
  return (await res.json()) as MySignUp;
}

/** signUp signs the player up: a linked one with no name (null), an unlinked one under their TTA username. */
export function signUp(base: string, slug: string, season: string, name: string | null, fetchFn: typeof fetch = fetch): Promise<Done> {
  return change(fetchFn, base, 'PUT', path(slug, season), name === null ? {} : { tta_name: name.trim() });
}

/** withdraw takes the player out, or their sign-up waiting for a link. */
export function withdraw(base: string, slug: string, season: string, fetchFn: typeof fetch = fetch): Promise<Done> {
  return change(fetchFn, base, 'DELETE', path(slug, season));
}
