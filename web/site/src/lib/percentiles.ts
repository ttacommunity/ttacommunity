/** The percentiles page in the browser: its address, the query it sends, and how it names bins, marks and standings. */
import type { PercentileQuery, PlayerPercentile, RatingBasis, RatingPercentiles } from '@ttac/shared/ratings';
import { ordinal } from '@ttac/shared/numbers';
import { DEFAULT_VIEW, modelOf, parseView, viewParams, type Remembered, type View } from './leaderboard-view';
import { RATING_TIERS, type RatingTier } from './player-view';
import type { Me } from './account';

/** The page's view: the leaderboard's model, format and filters it shares, the rating counted, the bins' width, and a player's name or ''. */
export interface PercentileView {
  board: Pick<View, 'engine' | 'reset' | 'format' | 'status' | 'minOpps' | 'title'>;
  rating: RatingBasis;
  binWidth: BinWidth;
  player: string;
}

/** The bin widths the API offers, in points. */
export const BIN_WIDTHS = [25, 50, 100] as const;
export type BinWidth = (typeof BIN_WIDTHS)[number];

/**
 * parsePercentileView reads the leaderboard's parameters the page uses, the rating, the bin width and the player,
 * falling back as the leaderboard does and to the conservative rating in 50-point bins.
 */
export function parsePercentileView(params: URLSearchParams, remembered: Remembered): PercentileView {
  const { engine, reset, format, status, minOpps, title } = parseView(params, remembered);
  const width = BIN_WIDTHS.find((w) => String(w) === params.get('bin_width'));
  return {
    board: { engine, reset, format, status, minOpps, title },
    rating: params.get('rating') === 'rating' ? 'rating' : 'c_rating',
    binWidth: width ?? 50,
    player: (params.get('player') ?? '').trim(),
  };
}

const boardOf = (v: PercentileView): View => ({ ...DEFAULT_VIEW, ...v.board });

/** withDefaults is Reset All's view: every option back to its default, keeping the format and the player, as the leaderboard keeps its format. */
export function withDefaults(v: PercentileView): PercentileView {
  const { engine, reset, status, minOpps, title } = DEFAULT_VIEW;
  return { board: { engine, reset, format: v.board.format, status, minOpps, title }, rating: 'c_rating', binWidth: 50, player: v.player };
}

/** percentileParams writes the view's address, leaving out what equals the leaderboard's defaults. */
export function percentileParams(v: PercentileView): URLSearchParams {
  const p = viewParams(boardOf(v));
  if (v.rating !== 'c_rating') p.set('rating', v.rating);
  if (v.binWidth !== 50) p.set('bin_width', String(v.binWidth));
  if (v.player !== '') p.set('player', v.player);
  return p;
}

/** percentileQuery is what the page asks the API for. */
export function percentileQuery(v: PercentileView): PercentileQuery {
  const { format, status, minOpps, title } = v.board;
  return { model: modelOf(boardOf(v)), format, status, minOpps, title, rating: v.rating, binWidth: v.binWidth, player: v.player };
}

/** leaderboardHref links to the leaderboard with the same filters, searching for the player. */
export function leaderboardHref(v: PercentileView): string {
  return `/ratings/?${viewParams({ ...boardOf(v), search: v.player })}`;
}

/** binLabel names a bin by its first and last whole rating, spaced when a negative number would blur the dash. */
export function binLabel(from: number, width: number): string {
  const to = from + width - 1;
  return from < 0 ? `${from} – ${to}` : `${from}–${to}`;
}

/** chartBins is the chart's bars: labels, counts, the player's bin (null without one in range), and the share of players below each bin in per cent. */
export function chartBins(r: RatingPercentiles): { labels: string[]; counts: number[]; playerBin: number | null; below: number[] } {
  const counts = r.bins.map((b) => b.count);
  const below: number[] = [];
  let sum = 0;
  for (const c of counts) {
    below.push(r.players > 0 ? (100 * sum) / r.players : 0);
    sum += c;
  }
  const c = r.player?.value;
  const at = c === undefined ? -1 : r.bins.findIndex((b) => c >= b.from && c < b.from + r.bin_width);
  return { labels: r.bins.map((b) => binLabel(b.from, r.bin_width)), counts, playerBin: at < 0 ? null : at, below };
}

/** topShare writes the player's rank as a share of the players, themself counted in: one place below 10%, whole above. */
export function topShare(p: Pick<PlayerPercentile, 'rank' | 'counted'>, players: number): string {
  const share = (100 * p.rank) / (players + (p.counted ? 0 : 1));
  return `Top ${share < 10 ? share.toFixed(1) : Math.round(share)}%`;
}

/** markLabel names a marked percentile: the 50th is the median. */
export function markLabel(percentile: number): string {
  return percentile === 50 ? 'Median' : `${ordinal(percentile)} percentile`;
}

/** ratingLabel names the rating counted, as the leaderboard heads its columns. */
export function ratingLabel(basis: RatingBasis): string {
  return basis === 'rating' ? 'Rating' : 'C-Rating';
}

/** tierOf is the rating tier, as the player's rating chart bands them, that a rating falls in. */
export function tierOf(rating: number): RatingTier {
  return RATING_TIERS.findLast((t, i) => i === 0 || rating >= t.min) ?? (RATING_TIERS[0] as RatingTier);
}

/** A tier's stretch of the chart, in bar positions: bar i spans i - 0.5 to i + 0.5. */
export interface TierSpan {
  tier: RatingTier;
  start: number;
  end: number;
}

/** tierSpans is each rating tier the bins reach, lowest first; the lowest and highest tiers have no bound. */
export function tierSpans(r: Pick<RatingPercentiles, 'bins' | 'bin_width'>): TierSpan[] {
  const first = r.bins[0];
  if (first === undefined) return [];
  const hi = first.from + r.bins.length * r.bin_width;
  const at = (rating: number) => (rating - first.from) / r.bin_width - 0.5;
  return RATING_TIERS.flatMap((tier, i) => {
    const next = RATING_TIERS[i + 1];
    const from = Math.max(i === 0 ? -Infinity : tier.min, first.from);
    const to = Math.min(next === undefined ? Infinity : next.min, hi);
    return from < to ? [{ tier, start: at(from), end: at(to) }] : [];
  });
}

/** withMe shows the signed-in player's own standing when the view names no player and they are linked to a rated one. */
export function withMe(view: PercentileView, me: Pick<Me, 'member'> | null): PercentileView {
  const mine = me?.member?.player ?? '';
  return view.player === '' && mine !== '' ? { ...view, player: mine } : view;
}
