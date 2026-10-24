import { describe, expect, it } from 'vitest';
import type { Player, PlayerHistory, Rival } from '@ttac/shared/ratings';
import {
  chartData,
  formatCards,
  isProvisional,
  matrixHref,
  matrixParams,
  parseMatrixView,
  pageModel,
  parsePlayerView,
  playerHref,
  playerParams,
  profileHref,
  ratingIn,
  score,
  signedDelta,
  runYears,
  seatName,
  seatDeltaClass,
  visibleRivals,
} from './player-view';

describe('playerHref', () => {
  it.each([
    ['a plain name', 'alder440', '/ratings/player/?name=alder440'],
    ['a space', 'gila loomer', '/ratings/player/?name=gila+loomer'],
    ['query characters', 'a&b=c', '/ratings/player/?name=a%26b%3Dc'],
  ])('puts the name in the query, encoded: %s', (_name, name, want) => {
    expect(playerHref(name)).toBe(want);
  });
});

describe('parsePlayerView', () => {
  it('reads the name, upstream options and the games page', () => {
    const p = new URLSearchParams('name=Ann&model=glicko2_std&reset_mode=softer&format=3&match_page=4');
    expect(parsePlayerView(p, {})).toEqual({ name: 'Ann', engine: 'glicko2_std', reset: 'softer', format: 3, matchPage: 4 });
  });

  it('falls back to remembered choices, then the defaults', () => {
    expect(parsePlayerView(new URLSearchParams('name=Ann'), { engine: 'glicko2_std', format: 2 })).toEqual({
      name: 'Ann',
      engine: 'glicko2_std',
      reset: 'continuous',
      format: 2,
      matchPage: 1,
    });
  });

  it('ignores a games page that is not a whole number from 1', () => {
    for (const bad of ['0', '-2', 'x', '1.5']) {
      expect(parsePlayerView(new URLSearchParams(`name=a&match_page=${bad}`), {}).matchPage).toBe(1);
    }
  });

  it('gives an empty name when none is asked for', () => {
    expect(parsePlayerView(new URLSearchParams(''), {}).name).toBe('');
  });
});

describe('playerParams', () => {
  const view = { name: 'Ann B', engine: 'glicko2_std', reset: 'softer', format: 4, matchPage: 1 } as const;

  it('always names the options, as upstream links do, and leaves out page 1', () => {
    expect(playerParams(view).toString()).toBe('name=Ann+B&model=glicko2_std&format=4&reset_mode=softer');
  });

  it('names a later games page', () => {
    expect(playerParams({ ...view, matchPage: 3 }).get('match_page')).toBe('3');
  });
});

describe('pageModel', () => {
  it('adds the season reset to GlickoD, which Standard does not have', () => {
    expect(pageModel('glicko2_daneo', 'continuous')).toBe('glicko2_daneo');
    expect(pageModel('glicko2_daneo', 'softer')).toBe('glicko2_daneo_softer');
    expect(pageModel('glicko2_std', 'softer')).toBe('glicko2_std');
  });
});

const player = (games: number, last: string | null, newest: string | null): Player => ({
  run_id: 1,
  name: 'Ann',
  country: null,
  title: null,
  peak: null,
  last_played: last,
  newest_game: newest,
  games: [
    { format: 0, games },
    { format: 2, games: 20 },
    { format: 3, games: 0 },
    { format: 4, games: 0 },
  ],
  ratings: [
    { model: 'glicko2_daneo_softer', format: 2, rank: 5, rating: 1700, rd: 60, c_rating: 1520, opponents: 20, win_rate: 50 },
  ],
});

describe('isProvisional', () => {
  // Provisional means under 15 games in the format while active; a missing date counts as 1900, as upstream.
  it.each([
    { name: '14 games, active', games: 14, last: '2026-01-01', newest: '2026-08-31', format: 0, want: true },
    { name: '15 games, active', games: 15, last: '2026-01-01', newest: '2026-08-31', format: 0, want: false },
    { name: '3 games in another format', games: 3, last: '2026-01-01', newest: '2026-08-31', format: 2, want: false },
    { name: 'a year to the day before the newest game', games: 3, last: '2025-08-31', newest: '2026-08-31', format: 0, want: true },
    { name: 'more than a year before the newest game', games: 3, last: '2025-08-30', newest: '2026-08-31', format: 0, want: false },
    { name: 'no dates at all', games: 3, last: null, newest: null, format: 0, want: true },
    { name: 'no last game, a newest game', games: 3, last: null, newest: '2026-08-31', format: 0, want: false },
  ] as const)('$name: provisional is $want', ({ games, last, newest, format, want }) => {
    expect(isProvisional(player(games, last, newest), format)).toBe(want);
  });
});

describe('ratingIn', () => {
  it('finds the rating for a model and format, or none', () => {
    const p = player(3, null, null);
    expect(ratingIn(p, 'glicko2_daneo_softer', 2)?.rank).toBe(5);
    expect(ratingIn(p, 'glicko2_std', 2)).toBeUndefined();
  });
});

describe('formatCards', () => {
  it("gives GlickoD's rating in every format, in upstream's order, for the reset mode chosen", () => {
    const p = player(3, null, null);
    p.ratings.push(
      { model: 'glicko2_daneo', format: 0, rank: 9, rating: 1800, rd: 50, c_rating: 1650, opponents: 40, win_rate: 55 },
      { model: 'glicko2_daneo_softer', format: 0, rank: 7, rating: 1810, rd: 51, c_rating: 1657, opponents: 40, win_rate: 55 },
      { model: 'glicko2_daneo', format: 3, rank: 2, rating: 1900, rd: 70, c_rating: 1690, opponents: 12, win_rate: 60 },
    );

    const cards = formatCards(p, 'continuous');

    expect(cards.map((c) => [c.format, c.label, c.rating?.rank ?? null])).toEqual([
      [0, 'Overall (All Formats)', 9],
      [2, '2-Player (Duels)', null],
      [3, '3-Player Tables', 2],
      [4, '4-Player Tables', null],
    ]);
    expect(formatCards(p, 'softer')[0].rating?.rank).toBe(7);
  });
});

describe('chartData', () => {
  const history: PlayerHistory = {
    run_id: 1,
    name: 'Ann',
    format: 0,
    series: [
      {
        model: 'glicko2_daneo',
        points: [
          { date: '2025-02-28', rating: 1810.04, c_rating: 1700.06 },
          { date: '2025-01-31', rating: 1800, c_rating: 1690 },
        ],
      },
      { model: 'glicko2_daneo_softer', points: [{ date: '2024-12-31', rating: 1500.55, c_rating: 1400 }] },
    ],
  };

  it('lines every engine up on every date any model has, rounded to 0.1', () => {
    const got = chartData(history, 'continuous', 'expected');
    expect(got.labels).toEqual(['2024-12-31', '2025-01-31', '2025-02-28']);
    expect(got.series.glicko2_daneo).toEqual([null, 1800, 1810]);
    expect(got.series.glicko2_std).toEqual([null, null, null]);
  });

  it('reads the season-reset models and the conservative curve; Standard has no season reset', () => {
    const got = chartData(history, 'softer', 'conservative');
    expect(got.series.glicko2_daneo).toEqual([1400, null, null]);
    expect(got.series.glicko2_std).toEqual([null, null, null]);
  });
});

describe('signedDelta', () => {
  it.each([
    [2.1, '+2.1'],
    [-3, '-3.0'],
    [0, '0.0'],
    [-0, '0.0'],
    [0.04, '0.0'],
    [-0.04, '0.0'],
    [0.05, '+0.1'],
    [null, '—'],
  ])('writes upstream ΔR: %s as %s', (delta, want) => {
    expect(signedDelta(delta)).toBe(want);
  });
});

describe('score', () => {
  it.each([
    [197, '197'],
    [-2, '-2'],
    [12.5, '12.5'],
  ])('writes %s as %s, dropping the decimals of a whole score', (n, want) => {
    expect(score(n)).toBe(want);
  });
});

const rival = (name: string, games: number, wins = 0, draws = 0): Rival => ({
  name,
  games,
  wins,
  losses: games - wins - draws,
  draws,
  win_rate: Math.round(((wins + draws / 2) / games) * 1000) / 10,
});

describe('visibleRivals', () => {
  const many = Array.from({ length: 35 }, (_, i) => rival(`p${String(i).padStart(2, '0')}`, 40 - i, i % 3));

  it('shows the first 30 until expanded', () => {
    expect(visibleRivals(many, { filter: '', minGames: 1, expanded: false, sort: null }).map((r) => r.name)).toHaveLength(30);
    expect(visibleRivals(many, { filter: '', minGames: 1, expanded: true, sort: null })).toHaveLength(35);
  });

  it('shows every match of a name filter or a minimum, past the first 30', () => {
    expect(visibleRivals(many, { filter: 'P3', minGames: 1, expanded: false, sort: null }).map((r) => r.name)).toEqual([
      'p30',
      'p31',
      'p32',
      'p33',
      'p34',
    ]);
    expect(visibleRivals(many, { filter: '', minGames: 38, expanded: false, sort: null })).toHaveLength(3);
  });

  it('sorts by a column, the first 30 of the new order shown', () => {
    const got = visibleRivals(many, { filter: '', minGames: 1, expanded: false, sort: { key: 'games', asc: true } });
    expect(got).toHaveLength(30);
    expect(got[0].name).toBe('p34');
    const byName = visibleRivals([rival('b', 1), rival('a', 1)], {
      filter: '',
      minGames: 1,
      expanded: false,
      sort: { key: 'name', asc: true },
    });
    expect(byName.map((r) => r.name)).toEqual(['a', 'b']);
  });
});

describe('matrix page address', () => {
  it('reads the name, model, reset mode, format and year, falling back to the remembered choices', () => {
    expect(parseMatrixView(new URLSearchParams('name=Ann&model=glicko2_std&format=3&reset_mode=softer&year=2024'), {})).toEqual({
      name: 'Ann',
      engine: 'glicko2_std',
      reset: 'softer',
      format: 3,
      year: 2024,
    });
    expect(parseMatrixView(new URLSearchParams('name=Ann'), { format: 2, reset: 'softer' })).toEqual({
      name: 'Ann',
      engine: 'glicko2_daneo',
      reset: 'softer',
      format: 2,
      year: null,
    });
  });

  it('ignores a year that is not a whole number from 2000 to 2100', () => {
    for (const bad of ['1999', '2101', 'x', '2024.5']) {
      expect(parseMatrixView(new URLSearchParams(`name=a&year=${bad}`), {}).year).toBeNull();
    }
  });

  it('writes the model, format and reset mode always and the year when chosen', () => {
    const view = { name: 'Ann B', engine: 'glicko2_daneo', reset: 'continuous', format: 0, year: null } as const;
    expect(matrixParams(view).toString()).toBe('name=Ann+B&model=glicko2_daneo&format=0&reset_mode=continuous');
    expect(matrixParams({ ...view, name: 'Ann', format: 4, year: 2019 }).toString()).toBe(
      'name=Ann&model=glicko2_daneo&format=4&reset_mode=continuous&year=2019',
    );
  });

  it("links to the matrix in the player page's model, reset mode and format", () => {
    expect(matrixHref('Ann', { engine: 'glicko2_std', reset: 'softer', format: 2 })).toBe(
      '/ratings/player/matrix/?name=Ann&model=glicko2_std&format=2&reset_mode=softer',
    );
  });

  it("links back to the player page in the matrix's model, reset mode and format", () => {
    expect(profileHref('Ann', { engine: 'glicko2_std', reset: 'softer', format: 2 })).toBe(
      '/ratings/player/?name=Ann&model=glicko2_std&format=2&reset_mode=softer',
    );
  });
});

describe('runYears', () => {
  it('gives the years of the first and last games', () => {
    expect(runYears('2017-03-04', '2026-08-31')).toEqual([2017, 2026]);
  });

  it('gives nothing when a date is missing', () => {
    expect(runYears(null, '2026-08-31')).toBeNull();
    expect(runYears('2017-03-04', null)).toBeNull();
  });
});

describe('seat helpers', () => {
  it('formats seat labels clearly', () => {
    expect(seatName(1)).toBe('Seat 1 (Starting)');
    expect(seatName(2)).toBe('Seat 2');
    expect(seatName(3)).toBe('Seat 3');
    expect(seatName(4)).toBe('Seat 4');
    expect(seatName(5)).toBe('Seat 5');
  });

  it('classifies seat delta', () => {
    expect(seatDeltaClass(3.5)).toBe('delta-pos');
    expect(seatDeltaClass(-2.1)).toBe('delta-neg');
    expect(seatDeltaClass(0)).toBe('delta-zero');
  });
});

