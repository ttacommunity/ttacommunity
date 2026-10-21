import { describe, expect, it, vi } from 'vitest';
import { confirmReport, fetchMyMatch, flagQuestion, flagReport, formFor, myMatchStatus, reportGame, yourRow, type MyBlitzMatch } from './blitz-report';

const match = (extra: Partial<MyBlitzMatch> = {}): MyBlitzMatch => ({
  id: 21,
  round: 3,
  a: 'Ann',
  b: 'Bob',
  me: 'b',
  result: null,
  game: null,
  ruled: false,
  report: null,
  reported_by: null,
  flag_reason: '',
  actions: ['report'],
  ...extra,
});
const game = { share_code: 'REP-1', a_culture: 50, a_outcome: 'finished', b_culture: 40, b_outcome: 'finished', finished_at: '2026-10-14T19:30:00Z' } as const;
const reported = (by: 'a' | 'b', report: MyBlitzMatch['report'], extra: Partial<MyBlitzMatch> = {}) => match({ result: 'a', game, report, reported_by: by, actions: [], ...extra });

describe('myMatchStatus', () => {
  it.each([
    ['not yet reported', match(), 'Not reported yet'],
    ['their own report', reported('b', 'reported', { actions: ['report'] }), 'You reported Ann won 50–40. Ann can confirm or flag it; until then you can correct it.'],
    ["the opponent's report", reported('a', 'reported', { actions: ['confirm', 'flag'] }), 'Ann reported Ann won 50–40. Confirm it if it is right, or flag it if not.'],
    ['confirmed', reported('a', 'confirmed'), 'Ann reported Ann won 50–40, and Bob confirmed it.'],
    ['flagged by them', reported('a', 'flagged', { flag_reason: 'I won' }), 'You flagged this report: “I won”. An organiser will resolve it.'],
    ['flagged by the opponent', reported('b', 'flagged', { flag_reason: 'I won' }), 'Ann flagged your report: “I won”. An organiser will resolve it.'],
    ['resolved', reported('a', 'resolved'), 'An organiser resolved this result: Ann won 50–40.'],
    ["an organiser's game", match({ result: 'a', game, actions: [] }), 'An organiser recorded this result: Ann won 50–40.'],
    ["an organiser's ruling", match({ result: 'b', ruled: true, actions: [] }), 'An organiser recorded this result: Bob won by forfeit.'],
    ['a bye', match({ b: null, me: 'a', actions: [] }), 'You have the bye this round.'],
    ['the next round paired', match({ actions: [] }), 'The next round is paired: ask an organiser to record this game.'],
  ])('%s', (_name, m, want) => {
    expect(myMatchStatus(m)).toContain(want);
  });
});

describe('flagQuestion', () => {
  it('asks for a reason, first suggesting the opponent correct it', () => {
    const q = flagQuestion(reported('a', 'reported'));
    expect(q.text).toContain('Ann');
    expect(q.text).toContain('correct');
    expect(q).toMatchObject({ required: true, maxLength: 500 });
  });
});

describe('formFor', () => {
  it("starts a new report empty, today, in the player's zone", () => {
    expect(formFor(match(), 'Europe/Berlin', '2026-10-14')).toMatchObject({ share_code: '', a_culture: '', finished_on: '2026-10-14', finished_time: '', zone: 'Europe/Berlin' });
  });
  it("fills a correction from the game reported, on the player's clocks", () => {
    expect(formFor(reported('b', 'reported'), 'Europe/Berlin', '2026-10-20')).toMatchObject({
      share_code: 'REP-1',
      a_culture: '50',
      b_culture: '40',
      finished_on: '2026-10-14',
      finished_time: '21:30',
    });
  });
  it.each([
    ['a region', 'US East'],
    ['Unknown', 'Unknown'],
  ])('leaves the time of a correction for the player to type in %s', (_name, zone) => {
    expect(formFor(reported('b', 'reported'), zone, '2026-10-20')).toMatchObject({ share_code: 'REP-1', finished_time: '', zone });
  });
});

describe('the requests', () => {
  const answering = (status: number, body: unknown = null) => vi.fn(async () => new Response(body === null ? null : JSON.stringify(body), { status }));
  it('reads the match with the session, null when signed out', async () => {
    const ok = answering(200, { match: match() });
    expect((await fetchMyMatch('https://api.example/', 'us_open', '2026', ok))?.id).toBe(21);
    expect(ok).toHaveBeenCalledWith('https://api.example/v1/me/blitz/us_open/2026', { credentials: 'include' });
    expect(await fetchMyMatch('https://api.example', 'us_open', '2026', answering(401, { error: 'not signed in' }))).toBeNull();
  });
  it.each([
    ['a report', (f: typeof fetch) => reportGame('https://api.example', 'us_open', '2026', 21, { share_code: 'REP-1', a: { culture: 50, outcome: 'finished' }, b: { culture: 40, outcome: 'finished' }, finished_on: '2026-10-14', finished_time: '21:30', zone: 'UTC' }, f), 'PUT', '/game'],
    ['a confirmation', (f: typeof fetch) => confirmReport('https://api.example', 'us_open', '2026', 21, f), 'POST', '/confirm'],
    ['a flag', (f: typeof fetch) => flagReport('https://api.example', 'us_open', '2026', 21, 'I won', f), 'POST', '/flag'],
  ])('sends %s with the session and answers a refusal with its message', async (_name, send, method, end) => {
    const ok = answering(204);
    expect(await send(ok as unknown as typeof fetch)).toEqual({ ok: true });
    expect(ok).toHaveBeenCalledWith(`https://api.example/v1/me/blitz/us_open/2026/matches/21${end}`, expect.objectContaining({ method, credentials: 'include' }));
    expect(await send(answering(409, { error: 'this result can no longer be changed here' }) as unknown as typeof fetch)).toEqual({ ok: false, message: 'this result can no longer be changed here' });
  });
});

describe('yourRow', () => {
  it.each([
    ['to report', match(), { round: 3, a: 'Ann', label: 'Your game: report it' }],
    ['their own report', reported('b', 'reported', { actions: ['report'] }), { round: 3, a: 'Ann', label: 'Your game: correct it' }],
    ["the opponent's report", reported('a', 'reported', { actions: ['confirm', 'flag'] }), { round: 3, a: 'Ann', label: 'Your game: confirm or flag it' }],
    ['nothing to do', reported('a', 'confirmed'), { round: 3, a: 'Ann', label: 'Your game' }],
  ])('%s', (_name, m, want) => {
    expect(yourRow(m)).toEqual(want);
  });
  it('is none without a match', () => {
    expect(yourRow(null)).toBeNull();
  });
});
