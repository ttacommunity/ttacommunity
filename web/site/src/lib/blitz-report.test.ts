import { describe, expect, it, vi } from 'vitest';
import { answerBestOf, bestOfStatus, confirmReport, fetchMyBlitz, flagQuestion, flagReport, formFor, reportGame, reportLines, yourRow, type MyBlitzGame, type MyBlitzMatch } from './blitz-report';

const match = (extra: Partial<MyBlitzMatch> = {}): MyBlitzMatch => ({
  id: 21,
  round: 3,
  a: 'Ann',
  b: 'Bob',
  me: 'b',
  result: null,
  ruled: false,
  best_of: 1,
  best_of_offered: false,
  proposed_by: null,
  best_of_actions: [],
  games: [],
  next_game: 1,
  ...extra,
});
const game = { share_code: 'REP-1', a_culture: 50, a_outcome: 'finished', a_turn: 2, b_culture: 40, b_outcome: 'finished', b_turn: 1, finished_at: '2026-10-14T19:30:00Z' } as const;
const played = (number: number, extra: Partial<MyBlitzGame> = {}): MyBlitzGame => ({ number, result: 'a', game, report: null, reported_by: null, flag_reason: '', actions: [], ...extra });
/** reported is a one-game match whose game side by reported, standing as report. */
const reported = (by: 'a' | 'b', report: MyBlitzGame['report'], extra: Partial<MyBlitzGame> = {}) =>
  match({ result: 'a', next_game: null, games: [played(1, { report, reported_by: by, ...extra })] });

describe('reportLines', () => {
  it.each([
    ['not yet reported', match(), 'Not reported yet', ['report']],
    ['their own report', reported('b', 'reported', { actions: ['report'] }), 'You reported Ann won 50–40. Ann can confirm or flag it; until then you can correct it.', ['report']],
    ["the opponent's report", reported('a', 'reported', { actions: ['confirm', 'flag'] }), 'Ann reported Ann won 50–40. Confirm it if it is right, or flag it if not.', ['confirm', 'flag']],
    ['confirmed', reported('a', 'confirmed'), 'Ann reported Ann won 50–40, and Bob confirmed it.', []],
    ['flagged by them', reported('a', 'flagged', { flag_reason: 'I won' }), 'You flagged this report: “I won”. An organiser will resolve it.', []],
    ['flagged by the opponent', reported('b', 'flagged', { flag_reason: 'I won' }), 'Ann flagged your report: “I won”. An organiser will resolve it.', []],
    ['resolved', reported('a', 'resolved'), 'An organiser resolved this result: Ann won 50–40.', []],
    ["an organiser's game", match({ result: 'a', next_game: null, games: [played(1)] }), 'An organiser recorded this result: Ann won 50–40.', []],
    ["an organiser's ruling", match({ result: 'b', ruled: true, next_game: null }), 'An organiser recorded this result: Bob won by forfeit.', []],
    ['a bye', match({ b: null, me: 'a', next_game: null }), 'You have the bye this round.', []],
    ['the next round paired', match({ next_game: null }), 'The next round is paired: ask an organiser to record this game.', []],
  ] as const)('reads a one-game match %s as one line', (_name, m, text, actions) => {
    const lines = reportLines(m);
    expect(lines).toHaveLength(1);
    expect(lines[0].heading).toBe('');
    expect(lines[0].text).toContain(text);
    expect(lines[0].actions).toEqual(actions);
  });

  it('gives a best of 3 a line for each game, and one for the next', () => {
    const m = match({
      best_of: 3,
      games: [played(1, { report: 'reported', reported_by: 'a', actions: ['confirm', 'flag'] }), played(2, { result: 'b', report: 'reported', reported_by: 'b', actions: ['report'] })],
      next_game: 3,
    });
    expect(reportLines(m).map((l) => [l.number, l.heading, l.actions])).toEqual([
      [1, 'Game 1', ['confirm', 'flag']],
      [2, 'Game 2', ['report']],
      [3, 'Game 3', ['report']],
    ]);
    expect(reportLines(m)[1].text).toContain('You reported Bob won 40–50');
    expect(reportLines(m)[2].text).toContain('Not reported yet');
  });

  it('says when a best of 3 is decided by a ruling', () => {
    const lines = reportLines(match({ best_of: 3, result: 'b', ruled: true, games: [played(1)], next_game: null }));
    expect(lines.at(-1)?.text).toBe('An organiser decided the match: Bob goes through.');
  });
});

describe('bestOfStatus', () => {
  it.each([
    ['a round of one-game matches', match(), ''],
    ['nothing proposed', match({ best_of_offered: true, best_of_actions: ['propose'] }), 'may be best of 3 if you and Ann both agree before game 1'],
    ['their own proposal', match({ best_of_offered: true, proposed_by: 'b', best_of_actions: ['withdraw'] }), 'You proposed best of 3. Ann can accept it until game 1 is reported.'],
    ["the opponent's proposal", match({ best_of_offered: true, proposed_by: 'a', best_of_actions: ['accept', 'decline'] }), 'Ann proposes best of 3'],
    ['agreed, one game each', match({ best_of: 3, games: [played(1), played(2, { result: 'b' })] }), 'Best of 3: Ann 1, Bob 1.'],
    ['decided by its games', match({ best_of: 3, result: 'b', games: [played(1, { result: 'b' }), played(2, { result: 'b' })] }), 'Best of 3: Ann 0, Bob 2. Bob goes through.'],
    ['settled as one game', match({ best_of_offered: true, games: [played(1)] }), ''],
  ])('%s', (_name, m, want) => {
    if (want === '') expect(bestOfStatus(m)).toBe('');
    else expect(bestOfStatus(m)).toContain(want);
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
    expect(formFor(null, 'Europe/Berlin', '2026-10-14')).toMatchObject({ share_code: '', a_culture: '', a_turn: null, b_turn: null, finished_on: '2026-10-14', finished_time: '', zone: 'Europe/Berlin' });
  });
  it("fills a correction from the game reported, on the player's clocks", () => {
    expect(formFor(game, 'Europe/Berlin', '2026-10-20')).toMatchObject({
      share_code: 'REP-1',
      a_culture: '50',
      a_turn: 2,
      b_culture: '40',
      b_turn: 1,
      finished_on: '2026-10-14',
      finished_time: '21:30',
    });
  });
  it.each([
    ['a region', 'US East'],
    ['Unknown', 'Unknown'],
  ])('leaves the time of a correction for the player to type in %s', (_name, zone) => {
    expect(formFor(game, zone, '2026-10-20')).toMatchObject({ share_code: 'REP-1', finished_time: '', zone });
  });
});

describe('the requests', () => {
  const answering = (status: number, body: unknown = null) => vi.fn(async () => new Response(body === null ? null : JSON.stringify(body), { status }));
  it('reads the match and the reports with the session, null when signed out', async () => {
    const reports = [{ round: 1, game: 1, a: 'Ann', reported_by: 'Ann', confirmed: false }];
    const ok = answering(200, { match: match(), reports });
    expect(await fetchMyBlitz('https://api.example/', 'us_open', '2026', ok)).toEqual({ match: match(), reports });
    expect(ok).toHaveBeenCalledWith('https://api.example/v1/me/blitz/us_open/2026', { credentials: 'include' });
    expect(await fetchMyBlitz('https://api.example', 'us_open', '2026', answering(401, { error: 'not signed in' }))).toBeNull();
  });
  it.each([
    ['a report', (f: typeof fetch) => reportGame('https://api.example', 'us_open', '2026', 21, 1, { share_code: 'REP-1', a: { culture: 50, outcome: 'finished', turn: 2 }, b: { culture: 40, outcome: 'finished', turn: 1 }, finished_on: '2026-10-14', finished_time: '21:30', zone: 'UTC' }, f), 'PUT', '/games/1', undefined],
    ['a confirmation', (f: typeof fetch) => confirmReport('https://api.example', 'us_open', '2026', 21, 2, f), 'POST', '/games/2/confirm', undefined],
    ['a flag', (f: typeof fetch) => flagReport('https://api.example', 'us_open', '2026', 21, 3, 'I won', f), 'POST', '/games/3/flag', '{"reason":"I won"}'],
    ['a best-of-3 answer', (f: typeof fetch) => answerBestOf('https://api.example', 'us_open', '2026', 21, 'accept', f), 'POST', '/best-of', '{"answer":"accept"}'],
  ])('sends %s with the session and answers a refusal with its message', async (_name, send, method, end, body) => {
    const ok = answering(204);
    expect(await send(ok as unknown as typeof fetch)).toEqual({ ok: true });
    expect(ok).toHaveBeenCalledWith(`https://api.example/v1/me/blitz/us_open/2026/matches/21${end}`, expect.objectContaining({ method, credentials: 'include' }));
    if (body !== undefined) expect(ok).toHaveBeenCalledWith(expect.any(String), expect.objectContaining({ body }));
    expect(await send(answering(409, { error: 'this result can no longer be changed here' }) as unknown as typeof fetch)).toEqual({ ok: false, message: 'this result can no longer be changed here' });
  });
});

describe('yourRow', () => {
  it.each([
    ['to report', match(), 'Your game: report it'],
    ['their own report', reported('b', 'reported', { actions: ['report'] }), 'Your game: correct it'],
    ["the opponent's report", reported('a', 'reported', { actions: ['confirm', 'flag'] }), 'Your game: confirm or flag it'],
    ["the opponent's proposal", match({ best_of_offered: true, proposed_by: 'a', best_of_actions: ['accept', 'decline'] }), 'Your game: answer the best-of-3 proposal'],
    ['the next game of a best of 3', match({ best_of: 3, games: [played(1, { report: 'confirmed', reported_by: 'a' })], next_game: 2 }), 'Your game: report it'],
    ['nothing to do', reported('a', 'confirmed'), 'Your game'],
  ])('%s', (_name, m, label) => {
    expect(yourRow(m)).toEqual({ round: 3, sideA: 'Ann', label });
  });
  it('is none without a match', () => {
    expect(yourRow(null)).toBeNull();
  });
});
