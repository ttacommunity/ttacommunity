import { describe, expect, it } from 'vitest';
import type { PlayerPercentile, RatingPercentiles } from '@ttac/shared/ratings';
import {
  binLabel,
  chartBins,
  leaderboardHref,
  markLabel,
  parsePercentileView,
  percentileParams,
  percentileQuery,
  ratingLabel,
  tierOf,
  tierSpans,
  topShare,
  withMe,
} from './percentiles';

const parse = (query: string, remembered = {}) => parsePercentileView(new URLSearchParams(query), remembered);

describe('parsePercentileView', () => {
  it('reads the leaderboard defaults and no player from an empty address', () => {
    expect(percentileQuery(parse(''))).toEqual({
      model: 'glicko2_daneo',
      format: 0,
      status: 'active',
      minOpps: 30,
      title: null,
      rating: 'c_rating',
      binWidth: 50,
      player: '',
    });
  });

  it('reads the leaderboard parameters it uses and the player', () => {
    const v = parse('model=glicko2_daneo&reset_mode=softer&format=3&status=all&min_opps=5&title=GM&player=%20ann%20');
    expect(percentileQuery(v)).toMatchObject({ model: 'glicko2_daneo_softer', format: 3, status: 'all', minOpps: 5, title: 'GM', player: 'ann' });
  });

  it.each([
    ['rating=rating&bin_width=25', 'rating', 25],
    ['bin_width=100', 'c_rating', 100],
    ['rating=elo&bin_width=30', 'c_rating', 50],
  ])('reads the rating counted and the bin width from %j, refusing unknown ones', (query, rating, binWidth) => {
    expect(percentileQuery(parse(query))).toMatchObject({ rating, binWidth });
  });

  it('falls back to the remembered engine and format', () => {
    expect(percentileQuery(parse('', { engine: 'glicko2_std', format: 2 }))).toMatchObject({ model: 'glicko2_std', format: 2 });
  });

  it('drops the leaderboard parameters the page has no use for', () => {
    expect(percentileParams(parse('search=ann&sort=name&page=3&player=bob')).toString()).toBe('player=bob');
  });
});

describe('percentileParams', () => {
  it.each([
    ['', ''],
    ['format=2&status=inactive', 'format=2&status=inactive'],
    ['player=ann+b', 'player=ann+b'],
    ['rating=c_rating&bin_width=50', ''],
    ['title=M&rating=rating&bin_width=25', 'title=M&rating=rating&bin_width=25'],
  ])('writes %j back as %j', (query, want) => {
    expect(percentileParams(parse(query)).toString()).toBe(want);
  });
});

describe('leaderboardHref', () => {
  it('links to the leaderboard with the same filters and the player searched', () => {
    expect(leaderboardHref(parse('format=3&min_opps=10&title=GM&rating=rating&bin_width=25&player=ann'))).toBe(
      '/ratings/?format=3&title=GM&min_opps=10&search=ann',
    );
  });
});

describe('binLabel', () => {
  it.each([
    [1500, 50, '1500–1549'],
    [-50, 50, '-50 – -1'],
  ])('names the bin from %d, %d wide, %j', (from, width, want) => {
    expect(binLabel(from, width)).toBe(want);
  });
});

describe('chartBins', () => {
  const spread = (player?: Partial<PlayerPercentile>): RatingPercentiles => ({
    run_id: 1,
    model: 'glicko2_daneo',
    format: 0,
    status: 'active',
    min_opps: 30,
    title: null,
    rating: 'c_rating',
    titles: [],
    players: 4,
    bin_width: 50,
    bins: [
      { from: 1500, count: 1 },
      { from: 1550, count: 0 },
      { from: 1600, count: 3 },
    ],
    marks: [],
    player: player && { name: 'ann', value: 0, rank: 1, percentile: 0, counted: true, ...player },
  });

  it("finds the player's bin", () => {
    expect(chartBins(spread({ value: 1649.9 })).playerBin).toBe(2);
    expect(chartBins(spread({ value: 1550 })).playerBin).toBe(1);
  });

  it('has no player bin without a player or outside the bins', () => {
    expect(chartBins(spread()).playerBin).toBeNull();
    expect(chartBins(spread({ value: 1700 })).playerBin).toBeNull();
  });

  it('counts the share of players below each bin', () => {
    expect(chartBins(spread()).below).toEqual([0, 25, 25]);
  });
});

describe('topShare', () => {
  it.each([
    [{ rank: 1, counted: true }, 1000, 'Top 0.1%'],
    [{ rank: 42, counted: true }, 1000, 'Top 4.2%'],
    [{ rank: 300, counted: true }, 1000, 'Top 30%'],
    [{ rank: 1, counted: false }, 0, 'Top 100%'],
    [{ rank: 2, counted: false }, 3, 'Top 50%'],
  ])('writes %o among %d players as %j, counting a left-out player in', (p, players, want) => {
    expect(topShare(p, players)).toBe(want);
  });
});

describe('markLabel', () => {
  it.each([
    [50, 'Median'],
    [90, '90th percentile'],
    [1, '1st percentile'],
  ])('names the %dth percentile %j', (p, want) => {
    expect(markLabel(p)).toBe(want);
  });
});

describe('ratingLabel', () => {
  it.each([
    ['c_rating', 'C-Rating'],
    ['rating', 'Rating'],
  ] as const)('names %j %j', (basis, want) => {
    expect(ratingLabel(basis)).toBe(want);
  });
});

describe('tierOf', () => {
  it.each([
    [-20, 'Bronze'],
    [1599.9, 'Silver'],
    [1600, 'Gold'],
    [2150, 'Grandmaster'],
    [3400, 'Supreme Grandmaster'],
  ])('puts %d in %j', (rating, want) => {
    expect(tierOf(rating).name).toBe(want);
  });
});

describe('tierSpans', () => {
  const bins = (from: number, n: number, width: number) => ({ bin_width: width, bins: Array.from({ length: n }, (_, i) => ({ from: from + i * width, count: 1 })) });
  const spans = (r: ReturnType<typeof bins>) => tierSpans(r).map((s) => [s.tier.name, s.start, s.end]);

  it.each([
    ['tiers met, in bar positions', bins(1500, 8, 50), [['Silver', -0.5, 1.5], ['Gold', 1.5, 5.5], ['Master', 5.5, 7.5]]],
    ['one tier', bins(1650, 2, 50), [['Gold', -0.5, 1.5]]],
    ['the lowest and highest tiers unbounded', bins(-100, 3, 100), [['Bronze', -0.5, 2.5]]],
    ['above the last tier', bins(3000, 2, 100), [['Supreme Grandmaster', -0.5, 1.5]]],
    ['no bins', bins(0, 0, 50), []],
  ])('spans %s', (_name, r, want) => {
    expect(spans(r)).toEqual(want);
  });
});

describe('withMe', () => {
  const linked = (player: string) => ({ member: { name: 'tta-name', player, banned: false, ban_reason: '' } });
  it.each([
    ['shows the signed-in player when none is asked for', '', linked('ann'), 'ann'],
    ['keeps the player asked for', 'bob', linked('ann'), 'bob'],
    ['shows nobody when signed out', '', null, ''],
    ['shows nobody when not linked', '', { member: null }, ''],
    ['shows nobody when linked with no rated games', '', linked(''), ''],
  ])('%s', (_name, player, me, want) => {
    expect(withMe({ ...parse(''), player }, me).player).toBe(want);
  });
});
