import { describe, expect, it } from 'vitest';
import type { LeaderboardRow, TitleCode, YearLeaderboard } from '@ttac/shared/ratings';
import {
  DEFAULT_VIEW,
  DELTA_DATES,
  csvUrl,
  INACTIVE_ON_OR_BEFORE,
  deltas,
  PER_PAGE,
  hasOptions,
  hasFilters,
  modelOf,
  nextSort,
  page,
  parseView,
  rankPool,
  search,
  sortRows,
  suggest,
  titleOptions,
  viewParams,
  yearDeltas,
  yearRows,
  type View,
} from './leaderboard-view';

function row(name: string, c_rating: number, opponents = 40, last_played: string | null = '2026-01-01', extra: Partial<LeaderboardRow> = {}): LeaderboardRow {
  return {
    rank: 1, rank_delta: 0, name, rating: c_rating + 100, rd: 30, c_rating, games: opponents, opponents,
    wins: 0, losses: 0, draws: 0, win_rate: 50, last_played, country: null, title: null, ...extra,
  };
}

const titled = (code: TitleCode) => ({ title: { code, count: 1, reason: '' } });
const view = (v: Partial<View> = {}): View => ({ ...DEFAULT_VIEW, ...v });

describe('defaults', () => {
  it('are upstream defaults', () => {
    expect(INACTIVE_ON_OR_BEFORE).toBe('2025-05-31');
    expect(PER_PAGE).toBe(50);
    expect(DEFAULT_VIEW).toEqual({
      engine: 'glicko2_daneo', reset: 'continuous', retro: false, format: 0, status: 'active', title: null,
      minOpps: 30, delta: 'none', search: '', sort: 'rank', order: 'asc', page: 1, year: null, stats: 'season',
    });
    expect(DELTA_DATES).toEqual({ last_update: null, month: '2026-05-01', quarter: '2026-03-01', year: '2025-05-31' });
  });
});

describe('parseView and viewParams', () => {
  it('reads upstream parameter names', () => {
    const got = parseView(
      new URLSearchParams('model=glicko2_std&reset_mode=softer&retro=1&format=3&status=all&title=GM&min_opps=5&delta=month&search=ann&sort=opps&order=desc&page=4'),
      {},
    );
    expect(got).toEqual({
      engine: 'glicko2_std', reset: 'softer', retro: true, format: 3, status: 'all', title: 'GM',
      minOpps: 5, delta: 'month', search: 'ann', sort: 'opps', order: 'desc', page: 4, year: null, stats: 'season',
    });
  });

  it('falls back to remembered choices, then defaults, for missing or unknown values', () => {
    const got = parseView(new URLSearchParams('model=elo&format=7&status=x&title=Q&min_opps=-3&delta=baseline&sort=x&order=x&page=0'), {
      engine: 'whr', reset: 'softer', retro: true, format: 2,
    });
    expect(got).toEqual({ ...DEFAULT_VIEW, engine: 'whr', reset: 'softer', retro: true, format: 2 });
  });

  it('lets the URL win over remembered choices', () => {
    expect(parseView(new URLSearchParams('model=glicko2_mp&format=0'), { engine: 'whr', format: 2 })).toMatchObject({
      engine: 'glicko2_mp',
      format: 0,
    });
  });

  it('writes only what differs from the defaults, and reads back the same view', () => {
    expect(viewParams(DEFAULT_VIEW).toString()).toBe('');
    const v = view({ engine: 'glicko2_adapt', status: 'inactive', minOpps: 0, delta: 'year', search: 'a, b', page: 2 });
    expect(viewParams(v).toString()).toBe('model=glicko2_adapt&status=inactive&min_opps=0&delta=year&search=a%2C+b&page=2');
    expect(parseView(viewParams(v), {})).toEqual(v);
  });

  it('always names the engine, reset mode and format in an explicit link, so memory cannot change it', () => {
    expect(viewParams(DEFAULT_VIEW, true).toString()).toBe('model=glicko2_daneo&reset_mode=continuous&retro=0&format=0');
    expect(parseView(viewParams(DEFAULT_VIEW, true), { engine: 'whr', reset: 'softer', retro: true, format: 2 })).toEqual(DEFAULT_VIEW);
  });
});

describe('modelOf', () => {
  it.each([
    [{}, 'glicko2_daneo'],
    [{ reset: 'softer' }, 'glicko2_daneo_softer'],
    [{ retro: true }, 'glicko2_daneo'],
    [{ engine: 'glicko2_std', retro: true }, 'glicko2_std_retro'],
    [{ engine: 'glicko2_mp', reset: 'softer', retro: true }, 'glicko2_mp_softer_retro'],
    [{ engine: 'whr', reset: 'softer', retro: true }, 'whr_softer'],
  ] as const)('composes %o into %s, as upstream', (v, model) => {
    expect(modelOf(view(v as Partial<View>))).toBe(model);
  });
});

describe('rankPool', () => {
  const rows = [
    row('active', 1800),
    row('few', 1990, 29),
    row('retired', 1900, 50, '2025-05-31'),
    row('never', 1950, 50, null),
    row('gm', 1700, 40, '2026-01-01', titled('GM')),
  ];

  it('keeps active players with enough opponents by default, ranked by conservative rating', () => {
    expect(rankPool(rows, view()).map((p) => [p.displayRank, p.name])).toEqual([
      [1, 'active'],
      [2, 'gm'],
    ]);
  });

  it('shows inactive players only, or everyone', () => {
    expect(rankPool(rows, view({ status: 'inactive' })).map((p) => p.name)).toEqual(['never', 'retired']);
    expect(rankPool(rows, view({ status: 'all', minOpps: 0 })).map((p) => p.name)).toEqual(['few', 'never', 'retired', 'active', 'gm']);
  });

  it('filters by title', () => {
    expect(rankPool(rows, view({ title: 'GM' })).map((p) => [p.displayRank, p.name])).toEqual([[1, 'gm']]);
  });

  it('marks retired and provisional players', () => {
    const got = rankPool([row('new', 1500, 14), row('old', 1400, 50, '2020-01-01')], view({ status: 'all', minOpps: 0 }));
    expect(got.map((p) => [p.name, p.active, p.provisional])).toEqual([
      ['new', true, true],
      ['old', false, false],
    ]);
  });
});

describe('search', () => {
  const pool = rankPool(
    [
      row('Annika', 1900, 40, '2026-01-01', { country: 'PL' }),
      row('ann', 1800),
      row('Bob', 1700, 40, '2026-01-01', { country: 'DE' }),
      row('Hannah', 1600),
    ],
    view(),
  );
  const names = (r: { rows: { name: string }[] }) => r.rows.map((p) => p.name);

  it('keeps everyone when there is no search', () => {
    expect(search(pool, '')).toEqual({ rows: pool, jumpTo: null });
  });

  it('jumps to an exact name, ignoring case, keeping the whole table', () => {
    expect(search(pool, 'ANN')).toEqual({ rows: pool, jumpTo: 'ann' });
  });

  it('jumps to the only match', () => {
    expect(search(pool, 'hann')).toEqual({ rows: pool, jumpTo: 'Hannah' });
  });

  it('shows only the matches of a partial name, with their own ranks', () => {
    const got = search(pool, 'nn');
    expect(names(got)).toEqual(['Annika', 'ann', 'Hannah']);
    expect(got.rows.map((p) => p.displayRank)).toEqual([1, 2, 4]);
    expect(got.jumpTo).toBeNull();
  });

  it('matches a country code exactly', () => {
    expect(search(pool, 'de')).toEqual({ rows: pool, jumpTo: 'Bob' });
  });

  it('shows the players of a comma-separated list', () => {
    expect(names(search(pool, 'bob, hann ,'))).toEqual(['Bob', 'Hannah']);
  });

  it('shows nobody when nothing matches', () => {
    expect(search(pool, 'zzz').rows).toEqual([]);
  });
});

describe('sortRows and nextSort', () => {
  const pool = rankPool([row('b', 1800, 60), row('a', 1900, 40), row('c', 1700, 50)], view());

  it('sorts by rank, name or a number, either way', () => {
    expect(sortRows(pool, 'rank', 'asc').map((p) => p.name)).toEqual(['a', 'b', 'c']);
    expect(sortRows(pool, 'rank', 'desc').map((p) => p.name)).toEqual(['c', 'b', 'a']);
    expect(sortRows(pool, 'name', 'asc').map((p) => p.name)).toEqual(['a', 'b', 'c']);
    expect(sortRows(pool, 'opps', 'desc').map((p) => p.name)).toEqual(['b', 'c', 'a']);
  });

  it('toggles as upstream: rank and name start ascending, numbers descending', () => {
    expect(nextSort(view(), 'rank')).toEqual({ sort: 'rank', order: 'desc' });
    expect(nextSort(view({ sort: 'rank', order: 'desc' }), 'rank')).toEqual({ sort: 'rank', order: 'asc' });
    expect(nextSort(view(), 'name')).toEqual({ sort: 'name', order: 'asc' });
    expect(nextSort(view(), 'c_rating')).toEqual({ sort: 'c_rating', order: 'desc' });
    expect(nextSort(view({ sort: 'c_rating', order: 'desc' }), 'c_rating')).toEqual({ sort: 'c_rating', order: 'asc' });
  });
});

describe('page', () => {
  const pool = rankPool(Array.from({ length: 120 }, (_, i) => row(`p${i}`, 2000 - i)), view());

  it('shows 50 rows a page, clamping the page asked for', () => {
    expect(page(pool, 2, null)).toMatchObject({ page: 2, pages: 3, total: 120 });
    expect(page(pool, 2, null).rows[0].name).toBe('p50');
    expect(page(pool, 9, null).page).toBe(3);
    expect(page([], 1, null)).toMatchObject({ page: 1, pages: 1, total: 0, rows: [] });
  });

  it('opens the page of the player jumped to', () => {
    expect(page(pool, 1, 'p105')).toMatchObject({ page: 3 });
  });
});

describe('hasOptions and hasFilters', () => {
  it('flags the options and filters that differ from upstream defaults', () => {
    expect(hasOptions(view())).toBe(false);
    expect(hasOptions(view({ reset: 'softer' }))).toBe(true);
    expect(hasOptions(view({ minOpps: 10 }))).toBe(true);
    expect(hasFilters(view({ engine: 'whr' }))).toBe(false);
    expect(hasFilters(view({ search: 'a' }))).toBe(true);
    expect(hasOptions(view({ delta: 'month' }))).toBe(true);
    expect(hasFilters(view({ delta: 'month' }))).toBe(true);
  });
});

describe('suggest', () => {
  const rows = ['Hannah', 'ann', 'Annika', 'Bob', 'Joanna'].map((n) => row(n, 1500));

  it('needs two characters, then ranks exact, then prefix, then substring matches', () => {
    expect(suggest(rows, 'a')).toEqual([]);
    expect(suggest(rows, 'AN').map((p) => p.name)).toEqual(['Annika', 'ann', 'Hannah', 'Joanna']);
    expect(suggest(rows, 'ann').map((p) => p.name)).toEqual(['ann', 'Annika', 'Hannah', 'Joanna']);
  });

  it('suggests for the last name of a comma-separated list, at most 12', () => {
    expect(suggest(rows, 'bob, jo').map((p) => p.name)).toEqual(['Joanna']);
    expect(suggest(Array.from({ length: 20 }, (_, i) => row(`xx${i}`, 1)), 'xx')).toHaveLength(12);
  });
});

describe('titleOptions', () => {
  it('lists the titles present, GM, M, P and G first, as upstream', () => {
    const rows = [row('a', 1, 40, null, titled('M')), row('b', 1, 40, null, titled('WC')), row('c', 1), row('d', 1, 40, null, titled('GM'))];
    expect(titleOptions(rows)).toEqual(['GM', 'M', 'WC']);
    expect(titleOptions([...rows, row('e', 1, 40, null, titled('SGM'))])).toEqual(['GM', 'M', 'SGM', 'WC']);
    expect(titleOptions([row('f', 1, 40, null, titled('B')), row('g', 1, 40, null, titled('W'))])).toEqual(['W', 'B']);
  });
});

describe('deltas', () => {
  it('is each shown player now minus their past rating, and 0 without history', () => {
    const got = deltas([row('ann', 1600), row('bob', 1500)], [{ name: 'ann', rating: 1650, rd: 30, c_rating: 1550 }]);
    expect(got.get('ann')).toEqual({ c_rating: 50, rating: 50 });
    expect(got.get('bob')).toEqual({ c_rating: 0, rating: 0 });
  });
});

describe('csvUrl', () => {
  it('asks the API for the table as shown', () => {
    const v = view({ engine: 'glicko2_std', reset: 'softer', format: 2, title: 'GM', search: 'a, b', sort: 'opps', order: 'desc', page: 3 });
    expect(csvUrl('https://api.example/', 7, v, false)).toBe(
      'https://api.example/v1/runs/7/leaderboard.csv?model=glicko2_std_softer&format=2&status=active&min_opps=30&title=GM&search=a%2C+b&sort=opps&order=desc',
    );
  });

  it('asks for everyone, inactive players too, when told to', () => {
    expect(csvUrl('https://api.example', 7, view({ minOpps: 50 }), true)).toBe(
      'https://api.example/v1/runs/7/leaderboard.csv?model=glicko2_daneo&format=0&status=all&min_opps=0&sort=rank&order=asc',
    );
  });

  it("asks for a year's archive in the record view shown", () => {
    expect(csvUrl('https://api.example', 7, view({ year: 2024, stats: 'career', minOpps: 0 }), false)).toBe(
      'https://api.example/v1/runs/7/leaderboard.csv?model=glicko2_daneo&format=0&status=active&min_opps=0&sort=rank&order=asc&year=2024&stats_view=career',
    );
  });
});

describe('year view', () => {
  it('reads the year and stats view, with 0 minimum opponents by default', () => {
    const got = parseView(new URLSearchParams('year=2024&stats_view=career'), {});
    expect(got).toMatchObject({ year: 2024, stats: 'career', minOpps: 0 });
    expect(parseView(new URLSearchParams('year=2024&min_opps=5'), {}).minOpps).toBe(5);
    for (const bad of ['1999', '2101', 'x']) expect(parseView(new URLSearchParams(`year=${bad}`), {}).year).toBeNull();
    expect(parseView(new URLSearchParams('year=2024&stats_view=x'), {}).stats).toBe('season');
  });

  it("writes the year, the career view, and a minimum other than the year view's 0", () => {
    const v = view({ year: 2024, minOpps: 0 });
    expect(viewParams(v).toString()).toBe('year=2024');
    expect(viewParams({ ...v, stats: 'career', minOpps: 30 }).toString()).toBe('min_opps=30&year=2024&stats_view=career');
  });

  it("counts the year view's own default as no filter", () => {
    expect(hasFilters(view({ year: 2024, minOpps: 0 }))).toBe(false);
    expect(hasFilters(view({ year: 2024, minOpps: 30 }))).toBe(true);
    expect(hasOptions(view({ year: 2024, minOpps: 0 }))).toBe(false);
  });

  const board: YearLeaderboard = {
    run_id: 2, year: 2024, model: 'glicko2_daneo', format: 0, snapshot: '2024-12-31', previous: '2023-12-31', games: 9,
    champions: [],
    players: [
      {
        name: 'ann', rating: 1690, rd: 30, c_rating: 1600, country: 'PL', title: null, last_played: '2024-12-20',
        season: { opponents: 3, wins: 0, losses: 2, draws: 1, win_rate: 16.7 },
        career: { opponents: 6, wins: 3, losses: 2, draws: 1, win_rate: 58.3 },
        previous: { rating: 1590, rd: 30, c_rating: 1500 },
      },
      {
        name: 'bob', rating: 1740, rd: 30, c_rating: 1650, country: null, title: null, last_played: null,
        season: { opponents: 0, wins: 0, losses: 0, draws: 0, win_rate: 0 },
        career: { opponents: 4, wins: 1, losses: 3, draws: 0, win_rate: 25 },
        previous: null,
      },
    ],
  };

  it('turns the board into rows with the season or career record, and marks who played that year', () => {
    const season = yearRows(board, 'season');
    expect(season.rows.map((r) => [r.name, r.opponents, r.wins, r.losses, r.draws, r.win_rate])).toEqual([
      ['ann', 3, 0, 2, 1, 16.7],
      ['bob', 0, 0, 0, 0, 0],
    ]);
    expect(yearRows(board, 'career').rows[0]).toMatchObject({ opponents: 6, wins: 3, win_rate: 58.3, last_played: '2024-12-20' });
    expect([...season.played]).toEqual(['ann']);
  });

  it('ranks the year view by who played that year, not by the live cutoff date', () => {
    const { rows, played } = yearRows(board, 'career');
    const active = rankPool(rows, view({ year: 2024, minOpps: 0 }), played);
    expect(active.map((p) => [p.name, p.displayRank, p.active, p.provisional])).toEqual([['ann', 1, true, true]]);
    const all = rankPool(rows, view({ year: 2024, minOpps: 0, status: 'all' }), played);
    expect(all.map((p) => [p.name, p.active])).toEqual([
      ['bob', false],
      ['ann', true],
    ]);
  });

  it("compares with the previous year's end, 0 for a player without one", () => {
    const got = yearDeltas(board);
    expect(got.get('ann')).toEqual({ c_rating: 100, rating: 100 });
    expect(got.get('bob')).toEqual({ c_rating: 0, rating: 0 });
  });
});
