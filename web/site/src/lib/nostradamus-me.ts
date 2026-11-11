/** A signed-in player's Nostradamus: reading their place and answers in a game, asking to play, and sending their answers to an open round. */
import type { MyNostradamus, NostradamusGiven } from '@ttac/shared/ratings';
import { AccountApiError, change, failureMessage, type Done } from './account';

/** MyPlace is the player's place in a game: signed out, signed in but not linked to a TTA username (or banned), or signed in with their status and answers. */
export type MyPlace = { kind: 'signed-out' } | { kind: 'not-linked' } | { kind: 'signed-in'; mine: MyNostradamus };

function url(base: string, path: string): string {
  return base.replace(/\/+$/, '') + path;
}

function gamePath(edition: string): string {
  return `/v1/me/nostradamus/${encodeURIComponent(edition)}`;
}

/** fetchMyNostradamus reads the signed-in player's place in the game. */
export async function fetchMyNostradamus(base: string, edition: string, fetchFn: typeof fetch = fetch): Promise<MyPlace> {
  const res = await fetchFn(url(base, gamePath(edition)), { credentials: 'include' });
  if (res.status === 401) return { kind: 'signed-out' };
  if (res.status === 403) return { kind: 'not-linked' };
  if (!res.ok) throw new AccountApiError(res.status, await failureMessage(res));
  return { kind: 'signed-in', mine: (await res.json()) as MyNostradamus };
}

/** askToPlay asks the organisers to let the player into the game. */
export function askToPlay(base: string, edition: string, fetchFn: typeof fetch = fetch): Promise<Done> {
  return change(fetchFn, base, 'POST', `${gamePath(edition)}/entry`);
}

/** sendAnswers replaces the player's answers in an open round. */
export function sendAnswers(base: string, edition: string, round: number, answers: NostradamusGiven[], fetchFn: typeof fetch = fetch): Promise<Done> {
  return change(fetchFn, base, 'PUT', `${gamePath(edition)}/rounds/${round}/answers`, { answers });
}
