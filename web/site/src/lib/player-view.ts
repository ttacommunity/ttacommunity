/** Upstream's player page in the browser: its address, options, chart series and rivals table. */
import { signed } from '@ttac/shared/numbers';
import type { Format, Model, Player, PlayerHistory, PlayerRating, Rival } from '@ttac/shared/ratings';
import { ENGINES, parseView, resetApplies, type Engine, type Remembered, type ResetMode } from './leaderboard-view';

/** Below this many games in a format an active player is provisional. */
const PROVISIONAL_BELOW = 15;

/** Rivals shown before "Show all". */
export const RIVALS_SHOWN = 30;

/** What the reader has chosen on a player's page, as upstream's query parameters carry it. */
export interface PlayerView {
  name: string;
  engine: Engine;
  reset: ResetMode;
  format: Format;
  matchPage: number;
}

/** playerHref returns the address of a player's page. */
export function playerHref(name: string): string {
  return `/ratings/player/?${new URLSearchParams({ name })}`;
}

/** searchHref returns where the header's search leads: a rated name typed in full (any case) to its page, anything else to the leaderboard's search. */
export function searchHref(names: readonly string[], term: string): string {
  const t = term.trim();
  const exact = names.find((n) => n.toLowerCase() === t.toLowerCase());
  return exact !== undefined ? playerHref(exact) : `/ratings/?${new URLSearchParams({ search: t })}`;
}

/** hasPlayerPage is false for the names the API gives no page: a deleted or banned player, and an empty seat. */
export function hasPlayerPage(name: string): boolean {
  return name !== 'Deleted player' && name !== 'Unknown player';
}

/** parsePlayerView reads the page's choices from its address, then remembered choices, then the defaults. */
export function parsePlayerView(params: URLSearchParams, remembered: Remembered): PlayerView {
  const { engine, reset, format } = parseView(params, remembered);
  const page = params.get('match_page') ?? '';
  return {
    name: params.get('name') ?? '',
    engine,
    reset,
    format,
    matchPage: /^\d+$/.test(page) && Number(page) >= 1 ? Number(page) : 1,
  };
}

/** playerParams writes the page's choices as upstream's links do, always naming the options. */
export function playerParams(view: PlayerView): URLSearchParams {
  const p = new URLSearchParams({ name: view.name, model: view.engine, format: String(view.format), reset_mode: view.reset });
  if (view.matchPage > 1) p.set('match_page', String(view.matchPage));
  return p;
}

/** pageModel returns the stored model for an engine and reset mode: the season reset is GlickoD's alone. */
export function pageModel(engine: Engine, reset: ResetMode): Model {
  return `${engine}${reset === 'softer' && resetApplies(engine) ? '_softer' : ''}` as Model;
}

/** ratingIn returns the player's rating in a model and format, if they have one. */
export function ratingIn(player: Player, model: Model, format: Format): PlayerRating | undefined {
  return player.ratings.find((r) => r.model === model && r.format === format);
}

/** FormatCard is one of upstream's GlickoD cards on a player's page: a format, how it is labelled, and the rating if any. */
export interface FormatCard {
  format: Format;
  label: string;
  icon: string;
  description: string;
  rating: PlayerRating | undefined;
}

const FORMAT_CARDS: Omit<FormatCard, 'rating'>[] = [
  { format: 0, label: 'Overall (All Formats)', icon: '👑', description: 'Combined competitive rating across all multiplayer and duel tables' },
  { format: 2, label: '2-Player (Duels)', icon: '⚔️', description: 'Direct head-to-head duel mastery and tactical tempo' },
  { format: 3, label: '3-Player Tables', icon: '👥', description: '3-player dynamic pacing and opportunistic scoring' },
  { format: 4, label: '4-Player Tables', icon: '🏰', description: 'Full 4-player diplomatic resilience and multi-threat navigation' },
];

/** formatCards returns the player's GlickoD rating in each format for the reset mode, as upstream's four cards show them. */
export function formatCards(player: Player, reset: ResetMode): FormatCard[] {
  const model = pageModel('glicko2_daneo', reset);
  return FORMAT_CARDS.map((c) => ({ ...c, rating: ratingIn(player, model, c.format) }));
}

const DAY = 86_400_000;
const day = (d: string | null) => Date.parse(`${d ?? '1900-01-01'}T00:00:00Z`);

/**
 * isProvisional applies upstream's rule: fewer than 15 games in the format, unless the player's last game is more
 * than a year before the newest game anyone played. A missing date counts as 1900, as upstream's does.
 */
export function isProvisional(player: Player, format: Format): boolean {
  const games = player.games.find((g) => g.format === format)?.games ?? 0;
  const idle = (day(player.newest_game) - day(player.last_played)) / DAY > 365;
  return games < PROVISIONAL_BELOW && !idle;
}

export type ChartMode = 'continuous' | 'softer' | 'all';
export type ChartCurve = 'expected' | 'conservative' | 'all';

export interface ChartSeriesLine {
  key: string;
  label: string;
  color: string;
  fill?: string;
  borderDash?: number[];
  curve: 'expected' | 'conservative';
  data: (number | null)[];
}

/** The chart's lines: one value per label and engine, null where the engine has none. */
export interface ChartData {
  labels: string[];
  series: Record<Engine, (number | null)[]>;
  lines: ChartSeriesLine[];
}

const tenth = (x: number) => Math.round(x * 10) / 10;

/**
 * chartData lines each engine's history up on every date any model has, in the continuous
 * or season-reset models and on the expected or conservative curve, or all selections at once.
 */
export function chartData(history: PlayerHistory, mode: ChartMode, curve: ChartCurve): ChartData {
  const labels = [...new Set(history.series.flatMap((s) => s.points.map((p) => p.date)))].sort();
  const suffix = mode === 'softer' ? '_softer' : '';
  const series = {} as ChartData['series'];
  for (const engine of ENGINES) {
    const points = history.series.find((s) => s.model === engine + suffix)?.points ?? [];
    const at = new Map(points.map((p) => [p.date, tenth(curve === 'conservative' ? p.c_rating : p.rating)]));
    series[engine] = labels.map((d) => at.get(d) ?? null);
  }

  const lines: ChartSeriesLine[] = [];
  const modes: ('continuous' | 'softer')[] = mode === 'all' ? ['continuous', 'softer'] : [mode];
  const curves: ('expected' | 'conservative')[] = curve === 'all' ? ['expected', 'conservative'] : [curve];

  for (const m of modes) {
    for (const engine of ENGINES) {
      if (engine === 'glicko2_std' && m === 'softer') continue;
      const mSuffix = m === 'softer' ? '_softer' : '';
      const modelSeries = history.series.find((s) => s.model === engine + mSuffix);
      if (!modelSeries || modelSeries.points.length === 0) continue;

      const points = modelSeries.points;
      const atRating = new Map(points.map((p) => [p.date, tenth(p.rating)]));
      const atCRating = new Map(points.map((p) => [p.date, tenth(p.c_rating)]));

      const engineName = engine === 'glicko2_daneo' ? '👑 GlickoD' : 'Glicko-2 Standard';
      const modeLabel = modes.length > 1 ? (m === 'softer' ? ' (Season Reset)' : ' (Continuous)') : '';
      const baseColor =
        engine === 'glicko2_daneo'
          ? m === 'softer'
            ? '#38bdf8'
            : '#c084fc'
          : '#e5a93c';
      const baseFill =
        engine === 'glicko2_daneo'
          ? m === 'softer'
            ? 'rgba(56, 189, 248, 0.12)'
            : 'rgba(168, 85, 247, 0.12)'
          : 'rgba(229, 169, 60, 0.1)';

      for (const c of curves) {
        const curveLabel = curves.length > 1 ? (c === 'conservative' ? ' (C)' : ' (E)') : '';
        const isDashed = c === 'conservative';
        lines.push({
          key: engine + '-' + m + '-' + c,
          label: engineName + modeLabel + curveLabel,
          color: baseColor,
          fill: isDashed ? 'transparent' : baseFill,
          borderDash: isDashed ? [5, 5] : undefined,
          curve: c,
          data: labels.map((d) => (c === 'conservative' ? atCRating.get(d) ?? null : atRating.get(d) ?? null)),
        });
      }
    }
  }

  return { labels, series, lines };
}
/** signedDelta writes a change as upstream writes ΔR: one decimal, a sign unless it rounds to zero, a dash for none. */
export function signedDelta(delta: number | null): string {
  return delta === null ? '—' : signed(delta, 1);
}

export type RivalKey = 'name' | 'games' | 'wins' | 'losses' | 'draws' | 'win_rate';

/** The rivals table's filter, minimum, expansion and sort. */
export interface RivalsView {
  filter: string;
  minGames: number;
  expanded: boolean;
  sort: { key: RivalKey; asc: boolean } | null;
}

/**
 * visibleRivals applies upstream's rivals table: sorted by the chosen column, the first 30 unless expanded; a name
 * filter or a minimum shows every match.
 */
export function visibleRivals(rivals: readonly Rival[], view: RivalsView): Rival[] {
  const sorted = [...rivals];
  if (view.sort !== null) {
    const { key, asc } = view.sort;
    const dir = asc ? 1 : -1;
    sorted.sort((a, b) => dir * (key === 'name' ? a.name.toLowerCase().localeCompare(b.name.toLowerCase()) : a[key] - b[key]));
  }
  const filter = view.filter.trim().toLowerCase();
  if (filter === '' && view.minGames <= 1) return view.expanded ? sorted : sorted.slice(0, RIVALS_SHOWN);
  return sorted.filter((r) => r.games >= view.minGames && r.name.toLowerCase().includes(filter));
}

/** firstSort is the direction a column starts in: names ascending, numbers descending. */
export function firstSort(key: RivalKey): boolean {
  return key === 'name';
}

/** What the reader has chosen on a player's matrix page; the engine comes from the player page. */
export interface MatrixView {
  name: string;
  engine: Engine;
  reset: ResetMode;
  format: Format;
  year: number | null;
}

/** parseMatrixView reads the matrix page's choices, falling back to the remembered ones. */
export function parseMatrixView(params: URLSearchParams, remembered: Remembered): MatrixView {
  const { engine, reset, format } = parseView(params, remembered);
  const y = params.get('year') ?? '';
  const year = /^\d{4}$/.test(y) && Number(y) >= 2000 && Number(y) <= 2100 ? Number(y) : null;
  return { name: params.get('name') ?? '', engine, reset, format, year };
}

/** matrixParams writes the matrix page's choices, the year only when one is chosen. */
export function matrixParams(view: MatrixView): URLSearchParams {
  const p = new URLSearchParams({ name: view.name, model: view.engine, format: String(view.format), reset_mode: view.reset });
  if (view.year !== null) p.set('year', String(view.year));
  return p;
}

/** matrixHref returns the address of a player's matrix page in the player page's model, reset mode and format. */
export function matrixHref(name: string, view: Pick<PlayerView, 'engine' | 'reset' | 'format'>): string {
  return `/ratings/player/matrix/?${matrixParams({ name, engine: view.engine, reset: view.reset, format: view.format, year: null })}`;
}

/** profileHref returns the address of a player's page in the matrix page's model, reset mode and format. */
export function profileHref(name: string, view: Pick<MatrixView, 'engine' | 'reset' | 'format'>): string {
  return `/ratings/player/?${playerParams({ name, engine: view.engine, reset: view.reset, format: view.format, matchPage: 1 })}`;
}

/** runYears returns the years of a run's first and last games, or null when either is missing. */
export function runYears(first: string | null, last: string | null): [number, number] | null {
  if (first === null || last === null) return null;
  return [Number(first.slice(0, 4)), Number(last.slice(0, 4))];
}

/** seatName returns a readable turn order label for seat number. */
export function seatName(seat: number): string {
  switch (seat) {
    case 1:
      return 'Seat 1 (Starting)';
    case 2:
      return 'Seat 2';
    case 3:
      return 'Seat 3';
    case 4:
      return 'Seat 4';
    default:
      return `Seat ${seat}`;
  }
}

/** seatDeltaClass returns the CSS class for a winrate delta: positive (green), negative (red), or neutral. */
export function seatDeltaClass(delta: number): 'delta-pos' | 'delta-neg' | 'delta-zero' {
  if (delta > 0) return 'delta-pos';
  if (delta < 0) return 'delta-neg';
  return 'delta-zero';
}

