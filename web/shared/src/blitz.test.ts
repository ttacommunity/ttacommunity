import { describe, expect, it } from 'vitest';
import { formatLabel, offsetLabel, resultSummary, zoneLabel, zoneOptions, type BlitzPlayedGame } from './blitz';

const game = (aCulture: number, bCulture: number, aOutcome: BlitzPlayedGame['a_outcome'] = 'finished', bOutcome: BlitzPlayedGame['b_outcome'] = 'finished'): BlitzPlayedGame => ({
  share_code: 'ABCD1234',
  a_culture: aCulture,
  a_outcome: aOutcome,
  b_culture: bCulture,
  b_outcome: bOutcome,
  played_on: '2026-10-14',
});

describe('resultSummary', () => {
  it.each([
    ['a bye', { a: 'Ann', b: null, result: null, game: null, ruled: false }, 'bye'],
    ['an undecided match', { a: 'Ann', b: 'Cy', result: null, game: null, ruled: false }, ''],
    ['a win', { a: 'Ann', b: 'Cy', result: 'a', game: game(250, 200), ruled: false }, 'Ann won 250–200'],
    ['a win for the second player', { a: 'Ann', b: 'Cy', result: 'b', game: game(200, 250), ruled: false }, 'Cy won 200–250'],
    ['a draw', { a: 'Ann', b: 'Cy', result: 'draw', game: game(210, 210), ruled: false }, 'Draw 210–210'],
    ['a resignation', { a: 'Ann', b: 'Cy', result: 'a', game: game(250, 0, 'finished', 'resigned'), ruled: false }, 'Ann won 250–0, Cy resigned'],
    ['a time-out', { a: 'Ann', b: 'Cy', result: 'b', game: game(0, 180, 'timed_out'), ruled: false }, 'Cy won 0–180, Ann timed out'],
    ['a ruling', { a: 'Ann', b: 'Cy', result: 'b', game: null, ruled: true }, 'Cy won by ruling'],
    ['a double loss', { a: 'Ann', b: 'Cy', result: 'double_loss', game: null, ruled: true }, 'Double loss by ruling'],
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

describe('zoneOptions', () => {
  it('puts UTC first, the rest in order, and the current zone in when the list lacks it', () => {
    expect(zoneOptions(['Europe/Berlin', 'UTC', 'America/New_York'], 'Asia/Tokyo')).toEqual([
      'UTC',
      'America/New_York',
      'Asia/Tokyo',
      'Europe/Berlin',
    ]);
  });
  it('does not repeat a current zone the list has, nor an empty one', () => {
    expect(zoneOptions(['Europe/Berlin'], 'Europe/Berlin')).toEqual(['UTC', 'Europe/Berlin']);
    expect(zoneOptions(['Europe/Berlin'], '')).toEqual(['UTC', 'Europe/Berlin']);
  });
});
