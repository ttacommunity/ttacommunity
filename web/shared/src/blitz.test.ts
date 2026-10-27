import { describe, expect, it } from 'vitest';
import { EDITION_COLUMNS, ENTRANT_COLUMNS, STANDING_COLUMNS, byFinish, offsetMinutes, formatLabel, knockoutStage, latestFinishes, placeLabel, offsetLabel, pairingSummary, pointsList, pointsText, reachedLabel, resultSummary, resultTag, roundShown, roundSummary, roundTitle, scoringSentence, seededName, turnMark, turnOrder, zoneLabel, type BlitzPlayedGame } from './blitz';
import { sortRows } from './sort';
import { editionHref, editionParams } from './blitz';

describe('editionParams', () => {
  it.each([
    ['both names', '?t=us_open&e=2026', { slug: 'us_open', edition: '2026' }],
    ['an edition with spaces', '?t=us_open&e=Season+1', { slug: 'us_open', edition: 'Season 1' }],
    ['no edition', '?t=us_open', null],
    ['an empty edition', '?t=us_open&e=', null],
    ['no tournament', '?e=2026', null],
    ['a slug that is not one', '?t=Bad!&e=2026', null],
    ['nothing', '', null],
  ])('reads %s', (_name, search, want) => {
    expect(editionParams(new URLSearchParams(search))).toEqual(want);
  });
});

describe('editionHref', () => {
  it('escapes the edition', () => {
    expect(editionHref('us_open', 'Season 1')).toBe('/blitz/?t=us_open&e=Season%201');
  });
});

const game = (aCulture: number, bCulture: number, aOutcome: BlitzPlayedGame['a_outcome'] = 'finished', bOutcome: BlitzPlayedGame['b_outcome'] = 'finished'): BlitzPlayedGame => ({
  share_code: 'ABCD1234',
  a_culture: aCulture,
  a_outcome: aOutcome,
  a_turn: 1,
  b_culture: bCulture,
  b_outcome: bOutcome,
  b_turn: 2,
  finished_at: '2026-10-14T19:30:00Z',
});

describe('turnMark', () => {
  it.each([
    [1, 'P1'],
    [2, 'P2'],
    [null, 'P?'],
  ] as const)('marks turn %s as %s', (turn, want) => {
    expect(turnMark(turn)).toBe(want);
  });
});

describe('turnOrder', () => {
  it.each([
    ['the first side first', { a: 'Ann', b: 'Cy', result: 'a', game: game(250, 200), ruled: false }, 'Turn order: P1 Ann, P2 Cy'],
    ['the second side first', { a: 'Ann', b: 'Cy', result: 'a', game: { ...game(250, 200), a_turn: 2, b_turn: 1 }, ruled: false }, 'Turn order: P1 Cy, P2 Ann'],
    ['a game recorded without it', { a: 'Ann', b: 'Cy', result: 'a', game: { ...game(250, 200), a_turn: null, b_turn: null }, ruled: false }, 'Turn order unknown'],
    ['no game', { a: 'Ann', b: 'Cy', result: 'b', game: null, ruled: true }, ''],
    ['a bye', { a: 'Ann', b: null, result: null, game: null, ruled: false }, ''],
  ] as const)('says %s', (_name, m, want) => {
    expect(turnOrder(m)).toBe(want);
  });
});

describe('resultSummary', () => {
  it.each([
    ['a bye', { a: 'Ann', b: null, result: null, game: null, ruled: false }, 'bye'],
    ['an undecided match', { a: 'Ann', b: 'Cy', result: null, game: null, ruled: false }, ''],
    ['a win', { a: 'Ann', b: 'Cy', result: 'a', game: game(250, 200), ruled: false }, 'Ann won 250–200'],
    ['a win for the second player', { a: 'Ann', b: 'Cy', result: 'b', game: game(200, 250), ruled: false }, 'Cy won 250–200'],
    ['a draw', { a: 'Ann', b: 'Cy', result: 'draw', game: game(210, 210), ruled: false }, 'Draw 210–210'],
    ['a drawn knockout game, the higher seed through', { a: 'Ann', b: 'Cy', result: 'b', game: game(180, 180), ruled: false }, 'Draw 180–180: Cy through as the higher seed'],
    ['a resignation', { a: 'Ann', b: 'Cy', result: 'a', game: game(250, 0, 'finished', 'resigned'), ruled: false }, 'Ann won 250–0, Cy resigned'],
    ['a time-out', { a: 'Ann', b: 'Cy', result: 'b', game: game(0, 180, 'timed_out'), ruled: false }, 'Cy won 180–0, Ann timed out'],
    ['a ruling with no game, a forfeit', { a: 'Ann', b: 'Cy', result: 'b', game: null, ruled: true }, 'Cy won by forfeit'],
    ['a double loss with no game', { a: 'Ann', b: 'Cy', result: 'double_loss', game: null, ruled: true }, 'Both forfeited'],
    ['a drawn ruling with no game', { a: 'Ann', b: 'Cy', result: 'draw', game: null, ruled: true }, 'Draw by ruling'],
    ['a ruling over a game', { a: 'Ann', b: 'Cy', result: 'draw', game: game(250, 200), ruled: true }, 'Draw by ruling (game: Ann 250, Cy 200)'],
    ['a win by ruling over a game the other won', { a: 'Ann', b: 'Cy', result: 'a', game: game(200, 250), ruled: true }, 'Ann won by ruling (game: Ann 200, Cy 250)'],
    ['a ruling over a resigned game', { a: 'Ann', b: 'Cy', result: 'b', game: game(250, 0, 'finished', 'resigned'), ruled: true }, 'Cy won by ruling (game: Ann 250, Cy 0, Cy resigned)'],
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

const FIDE = { win: 1, draw: 0.5, loss: 0, forfeit_win: 1, forfeit_loss: 0, bye: 1, skip: 0 };
const OURS = { win: 3, draw: 1.5, loss: 0.5, forfeit_win: 1, forfeit_loss: 0, bye: 2.5, skip: 10 };

describe('pointsText', () => {
  it.each([
    [0, '0'],
    [0.5, '½'],
    [1, '1'],
    [2.5, '2½'],
    [10, '10'],
  ])('writes %s as %s', (n, want) => {
    expect(pointsText(n)).toBe(want);
  });
});

describe('pointsList', () => {
  it('lists every outcome in order', () => {
    expect(pointsList(OURS)).toBe('win 3, draw 1½, loss ½, forfeit win 1, forfeit loss 0, bye 2½, skip 10');
  });
});

describe('scoringSentence', () => {
  it.each([
    ["FIDE's, as the site has always said", FIDE, true, 'A win is 1 point, a draw a half, a bye 1.'],
    ["the edition's own, every outcome", OURS, false, "This edition's points: win 3, draw 1½, loss ½, forfeit win 1, forfeit loss 0, bye 2½, skip 10."],
  ])('%s', (_, points, fide, want) => {
    expect(scoringSentence(points, fide)).toBe(want);
  });
});

describe('placeLabel', () => {
  it.each([
    ['a place held alone', 3, 3, '3'],
    ['a shared place', 1, 12, '1–12'],
  ] as const)('writes %s', (_name, from, to, want) => {
    expect(placeLabel({ place_from: from, place_to: to })).toBe(want);
  });
});

describe('latestFinishes', () => {
  const played = (a: string, b: string | null, at: string | null) => ({ a, b, game: at === null ? null : { finished_at: at } });
  it.each([
    ['each player of the latest Swiss round with a played game, by when it finished',
      [{ number: 1, kind: 'swiss', matches: [played('Ann', 'Cy', '2026-10-19T16:00:00Z')] }, { number: 2, kind: 'swiss', matches: [played('Ann', 'Di', '2026-10-26T10:00:00Z'), played('Cy', 'Bo', null)] }],
      { Ann: '2026-10-26T10:00:00Z', Di: '2026-10-26T10:00:00Z' }],
    ['the round before while the latest has no game yet',
      [{ number: 1, kind: 'swiss', matches: [played('Ann', 'Cy', '2026-10-19T16:00:00Z')] }, { number: 2, kind: 'swiss', matches: [played('Ann', 'Cy', null)] }],
      { Ann: '2026-10-19T16:00:00Z', Cy: '2026-10-19T16:00:00Z' }],
    ['no knockout round', [{ number: 7, kind: 'knockout', matches: [played('Ann', 'Cy', '2026-12-01T10:00:00Z')] }], {}],
    ['nobody for a bye', [{ number: 1, kind: 'swiss', matches: [played('Ann', null, null)] }], {}],
  ] as const)('finds %s', (_name, rounds, want) => {
    expect(Object.fromEntries(latestFinishes(rounds))).toEqual(want);
  });
});

describe('byFinish', () => {
  const row = (name: string, from: number) => ({ name, place_from: from });
  const finished = new Map([
    ['Ann', '2026-10-19T18:00:00Z'],
    ['Bo', '2026-10-19T16:00:00Z'],
    ['Di', '2026-10-19T17:00:00Z'],
  ]);
  it.each([
    ['earliest finish first within a shared place', [row('Ann', 1), row('Bo', 1), row('Di', 1)], ['Bo', 'Di', 'Ann']],
    ['players without a game after, in the order given', [row('Cy', 1), row('Ed', 1), row('Ann', 1)], ['Ann', 'Cy', 'Ed']],
    ['never across places', [row('Ann', 1), row('Bo', 2)], ['Ann', 'Bo']],
  ] as const)('keeps %s', (_name, rows, want) => {
    expect(byFinish(rows, finished).map((r) => r.name)).toEqual(want);
  });
});

describe('STANDING_COLUMNS', () => {
  const standing = (name: string, place: number, points: number, buchholz: number) => ({ rank: place, place_from: place, place_to: place, name, country: null, points, games: 1, byes: 0, buchholz, status: 'active' as const });
  const rows = [standing('cy', 1, 2, 1), standing('Ann', 2, 1, 3), standing('bo', 3, 1, 2)];
  it.each([
    ['place', { col: 'place', desc: false }, ['cy', 'Ann', 'bo']],
    ['player', { col: 'player', desc: false }, ['Ann', 'bo', 'cy']],
    ['points', { col: 'points', desc: true }, ['cy', 'Ann', 'bo']],
    ['buchholz', { col: 'buchholz', desc: true }, ['Ann', 'bo', 'cy']],
  ] as const)('sorts the standings by %s', (_name, sort, want) => {
    expect(sortRows(rows, STANDING_COLUMNS, sort).map((r) => r.name)).toEqual(want);
  });
});

describe('EDITION_COLUMNS', () => {
  const ed = (name: string, swiss: number, round: number | null, champion: string | null, entrants: number) => ({ name, edition: '2026', format: { swiss_rounds: swiss, knockout_size: 8 }, round, champion, entrants });
  const rows = [ed('B Open', 6, 3, null, 20), ed('a Cup', 8, null, null, 30), ed('C Open', 6, 6, 'Ann', 10)];
  it.each([
    ['tournament', { col: 'tournament', desc: false }, ['a Cup', 'B Open', 'C Open']],
    ['format, the longest first', { col: 'format', desc: true }, ['a Cup', 'B Open', 'C Open']],
    ['reached, a champion first, then the latest round, not started last', { col: 'reached', desc: true }, ['C Open', 'B Open', 'a Cup']],
    ['players', { col: 'players', desc: true }, ['a Cup', 'B Open', 'C Open']],
  ] as const)('sorts the editions by %s', (_name, sort, want) => {
    expect(sortRows(rows, EDITION_COLUMNS, sort).map((r) => r.name)).toEqual(want);
  });
});

describe('offsetMinutes', () => {
  it.each([
    ['+02:00', 120],
    ['-04:00', -240],
    ['+05:30', 330],
    ['', null],
  ] as const)('reads %s as %s', (offset, want) => {
    expect(offsetMinutes(offset)).toBe(want);
  });
});

describe('ENTRANT_COLUMNS', () => {
  const e = (name: string, zone: string, utc_offset: string, rating: number | null) => ({ name, zone, utc_offset, rating });
  const rows = [e('cy', 'Europe/Berlin', '+02:00', 1500), e('Ann', 'Unknown', '', null), e('bo', 'US East', '-04:00', 1800), e('di', 'Europe/Paris', '+02:00', 1600)];
  it.each([
    ['player', { col: 'player', desc: false }, ['Ann', 'bo', 'cy', 'di']],
    ['time zone, west to east, then by name, unknown last', { col: 'zone', desc: false }, ['bo', 'cy', 'di', 'Ann']],
    ['rating, the highest first, unrated last', { col: 'rating', desc: true }, ['bo', 'di', 'cy', 'Ann']],
  ] as const)('sorts the entrants by %s', (_name, sort, want) => {
    expect(sortRows(rows, ENTRANT_COLUMNS, sort).map((r) => r.name)).toEqual(want);
  });
});

describe('pairingSummary', () => {
  it.each([
    ['one method throughout', { first: 'dutch', later: 'dutch' }, "FIDE's Dutch system, every round"],
    ['round 1 and the later rounds', { first: 'random', later: 'dutch-timezone' }, 'Round 1: Random; later rounds: Dutch, time zones as tiebreak'],
  ] as const)('%s', (_, pairing, want) => {
    expect(pairingSummary(pairing)).toBe(want);
  });
});
