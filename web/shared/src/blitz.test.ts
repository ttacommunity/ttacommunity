import { describe, expect, it } from 'vitest';
import { formatLabel, knockoutStage, offsetLabel, reachedLabel, resultSummary, resultTag, roundShown, roundSummary, roundTitle, seededName, zoneLabel, type BlitzPlayedGame } from './blitz';

const game = (aCulture: number, bCulture: number, aOutcome: BlitzPlayedGame['a_outcome'] = 'finished', bOutcome: BlitzPlayedGame['b_outcome'] = 'finished'): BlitzPlayedGame => ({
  share_code: 'ABCD1234',
  a_culture: aCulture,
  a_outcome: aOutcome,
  b_culture: bCulture,
  b_outcome: bOutcome,
  finished_at: '2026-10-14T19:30:00Z',
});

describe('resultSummary', () => {
  it.each([
    ['a bye', { a: 'Ann', b: null, result: null, game: null, ruled: false }, 'bye'],
    ['an undecided match', { a: 'Ann', b: 'Cy', result: null, game: null, ruled: false }, ''],
    ['a win', { a: 'Ann', b: 'Cy', result: 'a', game: game(250, 200), ruled: false }, 'Ann won 250–200'],
    ['a win for the second player', { a: 'Ann', b: 'Cy', result: 'b', game: game(200, 250), ruled: false }, 'Cy won 200–250'],
    ['a draw', { a: 'Ann', b: 'Cy', result: 'draw', game: game(210, 210), ruled: false }, 'Draw 210–210'],
    ['a drawn knockout game, the higher seed through', { a: 'Ann', b: 'Cy', result: 'b', game: game(180, 180), ruled: false }, 'Draw 180–180: Cy through as the higher seed'],
    ['a resignation', { a: 'Ann', b: 'Cy', result: 'a', game: game(250, 0, 'finished', 'resigned'), ruled: false }, 'Ann won 250–0, Cy resigned'],
    ['a time-out', { a: 'Ann', b: 'Cy', result: 'b', game: game(0, 180, 'timed_out'), ruled: false }, 'Cy won 0–180, Ann timed out'],
    ['a ruling with no game, a forfeit', { a: 'Ann', b: 'Cy', result: 'b', game: null, ruled: true }, 'Cy won by forfeit'],
    ['a double loss with no game', { a: 'Ann', b: 'Cy', result: 'double_loss', game: null, ruled: true }, 'Both forfeited'],
    ['a drawn ruling with no game', { a: 'Ann', b: 'Cy', result: 'draw', game: null, ruled: true }, 'Draw by ruling'],
    ['a ruling over a game', { a: 'Ann', b: 'Cy', result: 'draw', game: game(250, 200), ruled: true }, 'Draw by ruling (game 250–200)'],
  ] as const)('summarises %s', (_name, m, want) => {
    expect(resultSummary(m)).toBe(want);
  });
});

describe('offsetLabel', () => {
  it.each([
    ['+02:00', 'UTC+2'],
    ['-04:00', 'UTC-4'],
    ['+05:30', 'UTC+5:30'],
    ['+00:00', 'UTC'],
    ['', ''],
  ])('shows %s as %s', (offset, want) => {
    expect(offsetLabel(offset)).toBe(want);
  });
});

describe('zoneLabel', () => {
  it('puts the offset before the zone', () => {
    expect(zoneLabel('Europe/Berlin', '+02:00')).toBe('UTC+2 · Europe/Berlin');
  });
  it('shows the zone alone when the offset is unknown', () => {
    expect(zoneLabel('Mars/Olympus', '')).toBe('Mars/Olympus');
  });
});

describe('formatLabel', () => {
  it.each([
    [{ swiss_rounds: 8, knockout_size: 8 }, '8 Swiss rounds, then a top-8 knockout'],
    [{ swiss_rounds: 1, knockout_size: 4 }, '1 Swiss round, then a top-4 knockout'],
    [{ swiss_rounds: 6, knockout_size: 0 }, '6 Swiss rounds, no knockout'],
  ])('describes %o', (format, want) => {
    expect(formatLabel(format)).toBe(want);
  });
});

describe('knockoutStage and roundTitle', () => {
  const usOpen = { swiss_rounds: 8, knockout_size: 8 };
  it.each([
    ['a Swiss round', 3, usOpen, null, 'Round 3'],
    ['the last Swiss round', 8, usOpen, null, 'Round 8'],
    ['the quarter-finals', 9, usOpen, 'Quarter-finals', 'Round 9 · Quarter-finals'],
    ['the semi-finals', 10, usOpen, 'Semi-finals', 'Round 10 · Semi-finals'],
    ['the final', 11, usOpen, 'Final', 'Round 11 · Final'],
    ['a round of sixteen', 6, { swiss_rounds: 5, knockout_size: 16 }, 'Round of 16', 'Round 6 · Round of 16'],
    ['a final straight after the Swiss', 2, { swiss_rounds: 1, knockout_size: 2 }, 'Final', 'Round 2 · Final'],
    ['a round past the final', 12, usOpen, null, 'Round 12'],
    ['a round past the Swiss with no knockout', 9, { swiss_rounds: 8, knockout_size: 0 }, null, 'Round 9'],
  ])('names %s', (_name, round, format, stage, title) => {
    expect(knockoutStage(round, format)).toBe(stage);
    expect(roundTitle(round, format)).toBe(title);
  });
});

describe('seededName', () => {
  it.each([
    ['a seeded player', 'Ann', 1, '(1) Ann'],
    ['a player without a seed', 'Ann', null, 'Ann'],
  ])('writes %s', (_name, name, seed, want) => {
    expect(seededName(name, seed)).toBe(want);
  });
});

describe('reachedLabel', () => {
  const usOpen = { swiss_rounds: 8, knockout_size: 8 };
  it.each([
    ['an edition not yet paired', null, null, 'Pairing not started'],
    ['a Swiss round', 3, null, 'Round 3'],
    ['a knockout round', 10, null, 'Round 10 · Semi-finals'],
    ['a finished edition', 11, 'Ann', 'Finished: champion Ann'],
  ])('says how far %s has got', (_name, round, champion, want) => {
    expect(reachedLabel(round, usOpen, champion)).toBe(want);
  });
});

describe('roundSummary', () => {
  const game = (result: string | null) => ({ b: 'Bob', result });
  const bye = { b: null, result: null };
  it.each([
    ['every game played, and the bye', [game('a'), game('draw'), bye], '2 games, all played, and a bye'],
    ['some still to play', [game('a'), game(null), game(null)], '3 games, 1 played'],
    ['none played', [game(null)], '1 game, none played'],
    ['no matches', [], 'no matches'],
  ])('sums up %s', (_name, matches, want) => {
    expect(roundSummary(matches)).toBe(want);
  });
});

describe('roundShown', () => {
  it.each([
    ['the newest round, untouched', 3, new Set<number>(), true],
    ['an earlier round, untouched', 2, new Set<number>(), false],
    ['the newest round, hidden by its button', 3, new Set([3]), false],
    ['an earlier round, shown by its button', 2, new Set([2]), true],
  ])('shows %s: %s', (_name, round, toggled, want) => {
    expect(roundShown(round, 3, toggled)).toBe(want);
  });
});

describe('resultTag', () => {
  it.each([
    ['a bye', { a: 'Ann', b: null, result: null, game: null, ruled: false }, 'Bye'],
    ['an undecided match', { a: 'Ann', b: 'Cy', result: null, game: null, ruled: false }, 'To be played'],
    ['a played game, which its scores tell', { a: 'Ann', b: 'Cy', result: 'a', game: game(250, 200), ruled: false }, ''],
    ['a drawn knockout game', { a: 'Ann', b: 'Cy', result: 'b', game: game(180, 180), ruled: false }, 'Cy through as the higher seed'],
    ['a forfeit', { a: 'Ann', b: 'Cy', result: 'b', game: null, ruled: true }, 'Cy won by forfeit'],
    ['both forfeiting', { a: 'Ann', b: 'Cy', result: 'double_loss', game: null, ruled: true }, 'Both forfeited'],
    ['a draw with no game', { a: 'Ann', b: 'Cy', result: 'draw', game: null, ruled: true }, 'Draw by ruling'],
    ['a ruling over a game', { a: 'Ann', b: 'Cy', result: 'a', game: game(200, 250), ruled: true }, 'Ann won by ruling'],
  ] as const)('tags %s', (_name, m, want) => {
    expect(resultTag(m)).toBe(want);
  });
});
