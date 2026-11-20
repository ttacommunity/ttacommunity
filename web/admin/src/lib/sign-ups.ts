/** What admin's sign-ups panel says: where a season's sign-ups stand, why they cannot open yet, and what each sign-up waiting for a link needs. */
import type { components } from '@ttac/shared/api';
import { formatInstant } from '@ttac/shared/time';

export type SignUps = components['schemas']['SignUps'];
export type PendingSignUp = components['schemas']['AdminPendingSignUp'];
export type AdminSignUps = components['schemas']['AdminSignUps'];

/** signUpsSummary says whether sign-ups are open, until when (in zone, named), and how many wait for a link. */
export function signUpsSummary(s: SignUps, zone: string): string {
  let line: string;
  if (!s.open) line = 'Closed.';
  else if (!s.taking) line = 'Open, but no longer taking entries: the closing time has passed or round 1 is paired.';
  else if (s.closes_at === null) line = 'Open: players sign up on the site until round 1 is paired.';
  else line = `Open: players sign up on the site until ${formatInstant(s.closes_at, zone)} or round 1, whichever comes first.`;
  return s.waiting === 0 ? line : `${line} ${s.waiting} waiting for their account to be linked.`;
}

/** openBlocker says why sign-ups cannot open, '' when they can: the site must show the tournament, and no round be paired. The API refuses both too. */
export function openBlocker(shown: boolean, rounds: number): string {
  if (!shown) return 'Show the tournament on the site first, under Tournaments.';
  if (rounds > 0) return 'Round 1 is paired: sign-ups can no longer open.';
  return '';
}

/** pendingNote says when a sign-up came (in zone, named) and what accepting it does, or what it needs first. */
export function pendingNote(p: PendingSignUp, zone: string): string {
  const when = `Signed up ${formatInstant(p.signed_up_at, zone)}.`;
  if (p.linked !== null) return `${when} Linked to ${p.linked} after sign-ups closed: Accept enters them.`;
  if (p.request !== null) return `${when} Accept approves its request to be linked to ${p.name}, and enters them.`;
  return `${when} Its request to be linked was answered: link account ${p.account} under Members first.`;
}

/** canAccept says whether accepting a sign-up can enter it: its account is linked by now, or has a request to be linked to approve. The API refuses the rest. */
export function canAccept(p: PendingSignUp): boolean {
  return p.linked !== null || p.request !== null;
}
