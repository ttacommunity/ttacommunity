import { describe, expect, it } from 'vitest';
import type { components } from '@ttac/shared/api';
import { blitzGames, editionViews, notPlaying, pairingNotes, ratingText, resultsUpdated, roundOptions, splitRounds } from './blitz';
import { PAIRING_METHODS } from '@ttac/shared/blitz';

describe('splitRounds', () => {
  it('puts the Swiss rounds newest first and the knockout rounds in bracket order, first to final', () => {
    const rounds = [
      { number: 1, kind: 'swiss' },
      { number: 4, kind: 'knockout' },
      { number: 3, kind: 'knockout' },
      { number: 2, kind: 'swiss' },
    ] as const;
    const { swiss, knockout } = splitRounds(rounds);
    expect(swiss.map((r) => r.number)).toEqual([2, 1]);
    expect(knockout.map((r) => r.number)).toEqual([3, 4]);
  });
});

describe('ratingText', () => {
  it.each([
    [1712.4, '1712'],
    [1699.5, '1700'],
    [null, '–'],
  ])('shows %s as %s', (rating, want) => {
    expect(ratingText(rating)).toBe(want);
  });
});

describe('editionViews', () => {
  it.each([
    ['nothing published: the players only', [], ['players']],
    ['a Swiss round published: the standings first, then the pairings and the players', [{ kind: 'swiss' }], ['standings', 'pairings', 'players']],
    ['the knockout begun: the same three', [{ kind: 'swiss' }, { kind: 'knockout' }], ['standings', 'pairings', 'players']],
  ] as const)('offers %s', (_name, rounds, want) => {
    expect(editionViews(rounds)).toEqual(want);
  });
});

type Round = components['schemas']['BlitzRound'];
type Match = components['schemas']['BlitzPublicMatch'];

const format = { swiss_rounds: 8, knockout_size: 8 };
const match = (a: string, b: string | null, extra: Partial<Match> = {}): Match => ({ a, a_rating: null, a_seed: null, b, b_rating: null, b_seed: null, result: null, game: null, ruled: false, reported_by: null, confirmed: false, ...extra });
const played = (code: string, finishedAt: string) => ({ share_code: code, a_culture: 250, a_outcome: 'finished' as const, a_turn: 1, b_culture: 0, b_outcome: 'timed_out' as const, b_turn: 2, finished_at: finishedAt });
const round = (number: number, matches: Match[], skipped: string[] = []): Round => ({ number, kind: 'swiss', method: 'timezone', pairing_order: 'rating', published_at: '2026-10-12T12:00:00Z', matches, skipped });
const rounds = [
  round(2, [
    match('Ann', 'Cy', { a_rating: 1712.4, result: 'a', game: played('ABCD1234', '2026-10-14T23:30:00Z') }),
    match('Dee', 'Eve', { result: 'b', ruled: true }),
    match('Fay', 'Gus'),
    match('Hal', null),
  ]),
  round(1, [match('Ann', 'Dee', { result: 'a', game: played('EFGH5678', '2026-10-07T20:00:00Z') })]),
];
const all = { round: null, player: '', coded: false, page: 1 };

describe('blitzGames sorted', () => {
  it.each([
    ['in the order given, unsorted', null, ['2-1', '2-2', '2-3', '2-4', '1-1']],
    ['by date, the latest first, unplayed last', { col: 'date', desc: true }, ['2-1', '1-1', '2-2', '2-3', '2-4']],
    ['by round, then game', { col: 'context', desc: false }, ['1-1', '2-1', '2-2', '2-3', '2-4']],
    ['by game number, then round', { col: 'game', desc: false }, ['1-1', '2-1', '2-2', '2-3', '2-4']],
    ['by players: round, then date, unplayed last', { col: 'players', desc: false }, ['1-1', '2-1', '2-2', '2-3', '2-4']],
    ['by replay code, none last', { col: 'code', desc: false }, ['2-1', '1-1', '2-2', '2-3', '2-4']],
  ] as const)('lists the games %s', (_name, sort, want) => {
    expect(blitzGames(rounds, format, all, 'UTC', null, sort).rows.map((r) => r.key)).toEqual(want);
  });
  it('breaks a tie on round and date by the highest score', () => {
    const at = '2026-10-14T23:30:00Z';
    const low = { ...played('', at), a_culture: 90, b_culture: 80 };
    const high = { ...played('', at), a_culture: 120, b_culture: 300 };
    const level = [round(1, [match('Ann', 'Cy', { result: 'a', game: low }), match('Bo', 'Di', { result: 'b', game: high })])];
    expect(blitzGames(level, format, all, 'UTC', null, { col: 'players', desc: false }).rows.map((r) => r.key)).toEqual(['1-2', '1-1']);
  });
});

describe('blitzGames', () => {
  it('turns a played match into a row: its date in the zone, round, game number, rated players linked with their ratings and turns, the scores, how it ended and the code', () => {
    expect(blitzGames(rounds, format, all, 'Australia/Sydney').rows[0]).toEqual({
      key: '2-1',
      date: '2026-10-15',
      context: 'Round 2',
      where: 'Game 1',
      tag: '',
      unplayed: false,
      seats: [
        { label: 'Ann', link: 'Ann', detail: '(1712) P1', score: 250, note: '' },
        { label: 'Cy', link: null, detail: 'P2', score: 0, note: 'timed out' },
      ],
      code: 'ABCD1234',
    });
  });
  it.each([
    ['the first side first', 1, 2, ['P1', 'P2']],
    ['the second side first', 2, 1, ['P2', 'P1']],
    ['a game recorded without them', null, null, ['P?', 'P?']],
  ] as const)("marks each player's turn, %s, in the match's order", (_name, aTurn, bTurn, want) => {
    const game = { ...played('TURN-1', '2026-10-14T23:30:00Z'), a_turn: aTurn, b_turn: bTurn };
    const marked = [round(1, [match('Ann', 'Cy', { result: 'a', game })])];
    expect(blitzGames(marked, format, all, 'UTC').rows[0].seats.map((s) => s.detail)).toEqual(want);
  });
  it('marks no turns on a match with no game', () => {
    expect(blitzGames(rounds, format, all, 'UTC').rows[2].seats.map((s) => s.detail)).toEqual(['', '']);
  });
  it.each([
    ['a forfeit, not played', 1, ['', 'Eve won by forfeit', true, null]],
    ['a match to be played', 2, ['', 'To be played', false, null]],
    ['a bye, not played', 3, ['', 'Bye', true, null]],
  ] as const)('tags %s', (_name, i, want) => {
    const row = blitzGames(rounds, format, all, 'UTC').rows[i];
    expect([row.date, row.tag, row.unplayed, row.code]).toEqual(want);
  });
  it.each([
    ['reported by a player', { reported_by: 'Ann' }, 'Reported by Ann'],
    ['reported and confirmed', { reported_by: 'Ann', confirmed: true }, 'Reported by Ann, confirmed'],
    ['entered by an organiser', {}, ''],
  ] as const)('tags a game %s', (_name, extra, want) => {
    const reported = [round(1, [match('Ann', 'Cy', { result: 'a', game: played('REP-1', '2026-10-14T23:30:00Z'), ...extra })])];
    expect(blitzGames(reported, format, all, 'UTC').rows[0].tag).toBe(want);
  });
  it.each([
    ['every round, in the order given', all, ['2-1', '2-2', '2-3', '2-4', '1-1'], 4, 2],
    ['one round', { ...all, round: 1 }, ['1-1'], 1, 1],
    ['one player', { ...all, player: 'Dee' }, ['2-2', '1-1'], 2, 1],
    ['games with a code', { ...all, coded: true }, ['2-1', '1-1'], 2, 2],
  ] as const)('narrows to %s, counting the games (byes aside) and those with a code', (_name, filter, keys, games, coded) => {
    const got = blitzGames(rounds, format, filter, 'UTC');
    expect([got.rows.map((r) => r.key), got.games, got.coded]).toEqual([keys, games, coded]);
  });
  it('shows a game recorded without a code with none, and leaves it out of those with a code', () => {
    const uncoded = [round(1, [match('Ann', 'Dee', { result: 'a', game: played('', '2026-10-07T20:00:00Z') })])];
    const got = blitzGames(uncoded, format, all, 'UTC');
    expect([got.rows[0].code, got.coded, blitzGames(uncoded, format, { ...all, coded: true }, 'UTC').rows.length]).toEqual([null, 0, 0]);
  });
  it('pages the rows', () => {
    const many = [round(1, Array.from({ length: 60 }, (_, i) => match(`P${i}`, `Q${i}`)))];
    const got = blitzGames(many, format, { ...all, page: 2 }, 'UTC');
    expect([got.rows.length, got.page, got.pages]).toEqual([10, 2, 2]);
  });
  it('names knockout players with their seeds', () => {
    const final = { number: 9, kind: 'knockout' as const, method: null, pairing_order: null, published_at: '2026-12-01T12:00:00Z', matches: [match('Ann', 'Cy', { a_seed: 1, b_seed: 2 })], skipped: [] };
    expect(blitzGames([final], { swiss_rounds: 8, knockout_size: 2 }, all, 'UTC').rows[0].seats.map((s) => s.label)).toEqual(['(1) Ann', '(2) Cy']);
  });
});

describe('notPlaying', () => {
  const sat = [round(3, [], ['Ivy']), round(2, [], []), round(1, [], ['Jo', 'Kim'])];
  it.each([
    ['one round', 1, [{ round: 'Round 1', names: ['Jo', 'Kim'] }]],
    ['a round nobody sat out', 2, []],
    ['every round, in the order given, those nobody sat out left out', null, [{ round: 'Round 3', names: ['Ivy'] }, { round: 'Round 1', names: ['Jo', 'Kim'] }]],
  ])('lists %s', (_name, filter, want) => {
    expect(notPlaying(sat, filter, format)).toEqual(want);
  });
});

describe('roundOptions', () => {
  it('names each round, in the order given', () => {
    expect(roundOptions(rounds, format)).toEqual([
      { value: 2, label: 'Round 2' },
      { value: 1, label: 'Round 1' },
    ]);
  });
});

describe('resultsUpdated', () => {
  it.each([
    ['a result recorded', '2026-10-14T19:30:00Z', 'Results last updated 14 Oct 2026, 21:30 (Europe/Berlin).'],
    ['no result yet', null, ''],
  ])('%s', (_name, at, want) => {
    expect(resultsUpdated(at, 'Europe/Berlin')).toBe(want);
  });
});

describe("blitzGames' row of the signed-in player's game", () => {
  it.each([
    ['their game in the latest round', { round: 2, sideA: 'Fay', label: 'Your game: report it' }, ['', '', 'Your game: report it', '']],
    ['no game of theirs', null, ['', '', '', '']],
  ] as const)('%s', (_name, you, want) => {
    expect(blitzGames(rounds, format, { ...all, round: 2 }, 'UTC', you).rows.map((r) => r.you ?? '')).toEqual(want);
  });
});

describe('pairingNotes', () => {
  const ed = (first: 'dutch' | 'random' | 'timezone', later: 'dutch' | 'rating', bye = 1, ranks: 'rating' | 'lot' = 'rating') => ({
    pairing: { first, later, ranks },
    points: { bye },
    format: { swiss_rounds: 8, knockout_size: 8 },
    rounds: [] as { number: number; kind: 'swiss' | 'knockout'; method: 'dutch' | 'timezone' | null }[],
  });
  it.each([
    ['one method for every round', ed('dutch', 'dutch'), [['Every Swiss round', 'dutch']]],
    ['round 1 and the later rounds', ed('random', 'rating'), [['Round 1', 'random'], ['Later rounds', 'rating']]],
  ] as const)('names the methods: %s', (_, edition, want) => {
    expect(pairingNotes(edition).methods).toEqual(want.map(([when, m]) => ({ when, label: PAIRING_METHODS[m].label, about: PAIRING_METHODS[m].about })));
  });
  it.each([
    ['a whole point', 1, 'worth 1 point'],
    ['half a point', 0.5, 'worth ½ point'],
    ['points', 3, 'worth 3 points'],
  ])('says what the bye is worth: %s', (_, bye, want) => {
    expect(pairingNotes(ed('dutch', 'dutch', bye)).rules.join(' ')).toContain(want);
  });
  it.each([
    ['by rating with the Dutch system', ed('dutch', 'dutch', 1, 'rating'), 'Starting ranks follow the ratings'],
    ['by lot with the Dutch system', ed('random', 'dutch', 1, 'lot'), 'Starting ranks are drawn'],
    ['not at all without the Dutch system', ed('timezone', 'rating', 1, 'lot'), null],
  ])('says how the starting ranks are given: %s', (_, edition, want) => {
    const rule = pairingNotes(edition).rules.find((r) => r.startsWith('Starting ranks')) ?? null;
    if (want === null) expect(rule).toBeNull();
    else expect(rule).toContain(want);
  });
  it('lists the method each Swiss round was paired with, the knockout left out', () => {
    const edition = ed('dutch', 'dutch');
    edition.rounds = [
      { number: 1, kind: 'swiss', method: 'timezone' },
      { number: 2, kind: 'swiss', method: 'dutch' },
      { number: 9, kind: 'knockout', method: null },
    ];
    expect(pairingNotes(edition).paired).toEqual([
      { round: 'Round 1', label: PAIRING_METHODS.timezone.label },
      { round: 'Round 2', label: PAIRING_METHODS.dutch.label },
    ]);
  });
});
