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
  ratingIn,
  score,
  signedDelta,
  runYears,
  visibleRivals,
} from './player-view';

describe('playerHref', () => {
  it('puts the name in the query, encoded', () => {
    expect(playerHref('a440')).toBe('/ratings/player/?name=a440');
    expect(playerHref('gigi lamoroso')).toBe('/ratings/player/?name=gigi+lamoroso');
    expect(playerHref('a&b=c')).toBe('/ratings/player/?name=a%26b%3Dc');
  });
});

describe('parsePlayerView', () => {
  it('reads the name, upstream options and the games page', () => {
    const p = new URLSearchParams('name=Ann&model=whr&reset_mode=softer&format=3&match_page=4');
    expect(parsePlayerView(p, {})).toEqual({ name: 'Ann', engine: 'whr', reset: 'softer', format: 3, matchPage: 4 });
  });

  it('falls back to remembered choices, then the defaults', () => {
    expect(parsePlayerView(new URLSearchParams('name=Ann'), { engine: 'glicko2_mp', format: 2 })).toEqual({
      name: 'Ann',
      engine: 'glicko2_mp',
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
  it('adds the season reset to the engine and never the retro prior', () => {
    expect(pageModel('glicko2_daneo', 'continuous')).toBe('glicko2_daneo');
    expect(pageModel('whr', 'softer')).toBe('whr_softer');
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
    { model: 'glicko2_std_softer', format: 2, rank: 5, rating: 1700, rd: 60, c_rating: 1520, opponents: 20, win_rate: 50 },
  ],
});

describe('isProvisional', () => {
  it('is true under 15 games in the format while active', () => {
    expect(isProvisional(player(14, '2026-01-01', '2026-08-31'), 0)).toBe(true);
    expect(isProvisional(player(15, '2026-01-01', '2026-08-31'), 0)).toBe(false);
    expect(isProvisional(player(3, '2026-01-01', '2026-08-31'), 2)).toBe(false);
  });

  it('is false after more than a year without a game, counted to the newest game', () => {
    expect(isProvisional(player(3, '2025-08-31', '2026-08-31'), 0)).toBe(true);
    expect(isProvisional(player(3, '2025-08-30', '2026-08-31'), 0)).toBe(false);
  });

  it('treats a missing date as 1900, as upstream does', () => {
    expect(isProvisional(player(3, null, null), 0)).toBe(true);
    expect(isProvisional(player(3, null, '2026-08-31'), 0)).toBe(false);
  });
});

describe('ratingIn', () => {
  it('finds the rating for a model and format, or none', () => {
    const p = player(3, null, null);
    expect(ratingIn(p, 'glicko2_std_softer', 2)?.rank).toBe(5);
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
      { model: 'whr_softer', points: [{ date: '2024-12-31', rating: 1500.55, c_rating: 1400 }] },
    ],
  };

  it('lines every engine up on every date any model has, rounded to 0.1', () => {
    const got = chartData(history, 'continuous', 'expected');
    expect(got.labels).toEqual(['2024-12-31', '2025-01-31', '2025-02-28']);
    expect(got.series.glicko2_daneo).toEqual([null, 1800, 1810]);
    expect(got.series.whr).toEqual([null, null, null]);
  });

  it('reads the season-reset models and the conservative curve', () => {
    const got = chartData(history, 'softer', 'conservative');
    expect(got.series.whr).toEqual([1400, null, null]);
    expect(got.series.glicko2_daneo).toEqual([null, null, null]);
  });
});

describe('signedDelta', () => {
  it('writes upstream ΔR: a sign when not zero, one decimal, a dash for none', () => {
    expect(signedDelta(2.1)).toBe('+2.1');
    expect(signedDelta(-3)).toBe('-3.0');
    expect(signedDelta(0)).toBe('0.0');
    expect(signedDelta(-0)).toBe('0.0');
    expect(signedDelta(null)).toBe('—');
  });
});

describe('score', () => {
  it('drops the decimals of a whole score', () => {
    expect(score(197)).toBe('197');
    expect(score(-2)).toBe('-2');
    expect(score(12.5)).toBe('12.5');
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
  it('reads the name, format and year, falling back to the remembered format', () => {
    expect(parseMatrixView(new URLSearchParams('name=Ann&format=3&year=2024'), {})).toEqual({ name: 'Ann', format: 3, year: 2024 });
    expect(parseMatrixView(new URLSearchParams('name=Ann'), { format: 2 })).toEqual({ name: 'Ann', format: 2, year: null });
  });

  it('ignores a year that is not a whole number from 2000 to 2100', () => {
    for (const bad of ['1999', '2101', 'x', '2024.5']) {
      expect(parseMatrixView(new URLSearchParams(`name=a&year=${bad}`), {}).year).toBeNull();
    }
  });

  it('writes the format always and the year when chosen', () => {
    expect(matrixParams({ name: 'Ann B', format: 0, year: null }).toString()).toBe('name=Ann+B&format=0');
    expect(matrixParams({ name: 'Ann', format: 4, year: 2019 }).toString()).toBe('name=Ann&format=4&year=2019');
    expect(matrixHref('Ann', 2)).toBe('/ratings/player/matrix/?name=Ann&format=2');
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
