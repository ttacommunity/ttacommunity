import { describe, expect, it } from 'vitest';
import { canAccept, openBlocker, pendingNote, signUpsSummary, type PendingSignUp, type SignUps } from './sign-ups';

const ZONE = 'Australia/Sydney';
const state = (extra: Partial<SignUps> = {}): SignUps => ({ open: false, closes_at: null, taking: false, waiting: 0, ...extra });

describe('signUpsSummary', () => {
  it.each([
    ['closed', state(), 'Closed.'],
    ['open, no closing time', state({ open: true, taking: true }), 'Open: players sign up on the site until round 1 is paired.'],
    ['open, closing', state({ open: true, taking: true, closes_at: '2026-12-01T07:00:00Z' }), 'Open: players sign up on the site until 1 Dec 2026, 18:00 (Australia/Sydney) or round 1, whichever comes first.'],
    ['open, past its closing time or round 1', state({ open: true, closes_at: '2026-12-01T07:00:00Z' }), 'Open, but no longer taking entries: the closing time has passed or round 1 is paired.'],
    ['with sign-ups waiting', state({ open: true, taking: true, waiting: 2 }), 'Open: players sign up on the site until round 1 is paired. 2 waiting for their account to be linked.'],
  ])('%s', (_name, s, want) => {
    expect(signUpsSummary(s, ZONE)).toBe(want);
  });
});

describe('openBlocker', () => {
  it.each([
    ['shown, no round', true, 0, ''],
    ['hidden', false, 0, 'Show the tournament on the site first, under Tournaments.'],
    ['round 1 paired', true, 1, 'Round 1 is paired: sign-ups can no longer open.'],
  ])('%s', (_name, shown, rounds, want) => {
    expect(openBlocker(shown, rounds)).toBe(want);
  });
});

describe('pendingNote', () => {
  const pending = (extra: Partial<PendingSignUp> = {}): PendingSignUp => ({ id: 3, account: 41, name: 'Newcomer', signed_up_at: '2026-10-20T09:00:00Z', request: 12, linked: null, ...extra });
  it.each([
    ['a link request waiting', pending(), 'Signed up 20 Oct 2026, 20:00 (Australia/Sydney). Accept approves its request to be linked to Newcomer, and enters them.'],
    ['linked since', pending({ request: null, linked: 'Newcomer' }), 'Signed up 20 Oct 2026, 20:00 (Australia/Sydney). Linked to Newcomer after sign-ups closed: Accept enters them.'],
    ['no request waiting', pending({ request: null }), 'Signed up 20 Oct 2026, 20:00 (Australia/Sydney). Its request to be linked was answered: link account 41 under Members first.'],
  ])('%s', (_name, p, want) => {
    expect(pendingNote(p, ZONE)).toBe(want);
  });
});

describe('canAccept', () => {
  const pending = (request: number | null, linked: string | null): PendingSignUp => ({ id: 3, account: 41, name: 'Newcomer', signed_up_at: '2026-10-20T09:00:00Z', request, linked });
  it.each([
    ['a link request waiting', pending(12, null), true],
    ['linked since', pending(null, 'Newcomer'), true],
    ['neither', pending(null, null), false],
  ])('%s', (_name, p, want) => {
    expect(canAccept(p)).toBe(want);
  });
});
