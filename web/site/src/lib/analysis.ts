/** Upstream's model analysis in the browser: its model list, composite score, ranking and curve filters. */
import { signed } from '@ttac/shared/numbers';
import { titleName } from './leaderboard-display';
import type { Activity, CalibrationRow, CountryRatings, Format, SeatStatsGroup, TitleCode } from '@ttac/shared/ratings';

export type { CountryRatings };

export type Family = 'glicko2_daneo' | 'glicko2_std';
export type Reset = 'continuous' | 'softer';
export type EvalMode = CalibrationRow['mode'];

export interface ModelInfo {
  key: CalibrationRow['model'];
  name: string;
  family: Family;
  reset: Reset;
  retro: boolean;
  color: string;
  bg: string;
  verdict: string;
}

const m = (
  key: ModelInfo['key'],
  name: string,
  family: Family,
  reset: Reset,
  retro: boolean,
  color: string,
  bg: string,
  verdict: string,
): ModelInfo => ({ key, name, family, reset, retro, color, bg, verdict });

/** MODELS is upstream's MODEL_METADATA, in its order (ties in the ranking keep it). */
export const MODELS: readonly ModelInfo[] = [
  m('glicko2_daneo', 'GlickoD (Continuous)', 'glicko2_daneo', 'continuous', true, '#8b5cf6', 'rgba(139, 92, 246, 0.25)', '★ The Premier Flagship — Golden ratio multiplayer information decomposition (φ^(N-2)), 15-game re-calibration, and bilateral composite variance.'),
  m('glicko2_daneo_softer', 'GlickoD (Season Reset)', 'glicko2_daneo', 'softer', true, '#a855f7', 'rgba(168, 85, 247, 0.25)', '★ Peak Statistical & Competitive Harmony — Golden Ratio decomposition with Season Reset (α ≈ 1.45, λ ≈ 0.12), 15-game re-calibration, and all-time record 0.24% ECE.'),
  m('glicko2_std', 'Glicko-2 Standard (Continuous)', 'glicko2_std', 'continuous', false, '#ffca28', 'rgba(255, 202, 40, 0.2)', '★ High Forward Responsiveness — Standard pairwise Elo dynamics; classical odometer benchmark.'),
];

const clamp = (x: number) => Math.max(0, Math.min(1, x));
const fixed = (x: number, places: number) => Number(x.toFixed(places));

/** compositeScore is upstream's 0–100 index: Brier and ECE 35% each, log loss and accuracy 15% each. */
export function compositeScore(brier: number, ece: number, logLoss: number, accuracy: number): number {
  const b = clamp((0.25 - brier) / 0.1);
  const e = clamp((0.08 - ece) / 0.08);
  const l = clamp((0.7 - logLoss) / 0.25);
  const a = clamp((accuracy - 50) / 30);
  return fixed(100 * (0.35 * b + 0.35 * e + 0.15 * l + 0.15 * a), 1);
}

export type RankedModel = ModelInfo & {
  rank: number;
  brier: number;
  ece: number;
  ecePct: number;
  logLoss: number;
  accuracy: number;
  pIstPct: number;
  pSollPct: number;
  pDeltaPct: number;
  composite: number;
  actuals: number[];
};

/**
 * rankModels ranks every model for an evaluation mode and format by composite score, best first, with the favourite's
 * actual (Ist) and predicted (Soll) win rate averaged over the bins. A model without a row gets upstream's
 * placeholder figures.
 */
export function rankModels(rows: readonly CalibrationRow[], mode: EvalMode, format: Format): RankedModel[] {
  const ranked = MODELS.map((info): RankedModel => {
    const r = rows.find((x) => x.mode === mode && x.format === format && x.model === info.key);
    if (r === undefined) {
      const composite = compositeScore(0.214, 0.015, 0.62, 64.5);
      return { ...info, rank: 0, brier: 0.214, ece: 0.015, ecePct: 1.5, logLoss: 0.62, accuracy: 64.5, pIstPct: 65, pSollPct: 65, pDeltaPct: 0, composite, actuals: [] };
    }
    const { preds, actuals, counts } = r.bins;
    const total = counts.reduce((s, c) => s + c, 0);
    const soll = total > 0 ? preds.reduce((s, p, i) => s + p * (counts[i] ?? 0), 0) / total : 0.65;
    const ist = total > 0 ? actuals.reduce((s, a, i) => s + a * (counts[i] ?? 0), 0) / total : 0.65;
    const brier = fixed(r.brier, 4);
    const ece = fixed(r.ece, 4);
    const logLoss = r.log_loss === null ? 0.62 : fixed(r.log_loss, 4);
    const accuracy = r.accuracy === null ? 64 : fixed(r.accuracy, 1);
    return {
      ...info,
      rank: 0,
      brier,
      ece,
      ecePct: fixed(r.ece * 100, 2),
      logLoss,
      accuracy,
      pIstPct: fixed(ist * 100, 1),
      pSollPct: fixed(soll * 100, 1),
      pDeltaPct: fixed((ist - soll) * 100, 1),
      composite: compositeScore(brier, ece, logLoss, accuracy),
      actuals,
    };
  });
  return ranked.sort((a, b) => b.composite - a.composite).map((x, i) => ({ ...x, rank: i + 1 }));
}

/** The analysis selector's engine and reset filters. */
export interface AnalysisFilters {
  engines: ReadonlySet<Family>;
  resets: ReadonlySet<Reset>;
}

/** activeModels are the curves drawn: the chosen engines in the chosen reset modes. The table lists every model, drawn ones highlighted. */
export function activeModels(f: AnalysisFilters): ModelInfo['key'][] {
  return MODELS.filter((i) => f.engines.has(i.family) && f.resets.has(i.reset)).map((i) => i.key);
}

/** eceClass is upstream's colour for an ECE percentage. */
export function eceClass(pct: number): 'good' | 'fair' | 'poor' {
  return pct < 1.5 ? 'good' : pct < 2.5 ? 'fair' : 'poor';
}

export const FORMAT_NAMES: Record<Format, string> = { 0: 'All Formats', 2: '2-Player (Duel)', 3: '3-Player', 4: '4-Player' };
export const MODE_NAMES: Record<EvalMode, string> = { standard: 'Standard (Match-Time)', walk_forward: 'Walk-Forward (Out-of-Sample)' };

/** parseAnalysisParams reads the address's eval_mode and format, falling back to the remembered format, as upstream's session. */
export function parseAnalysisParams(params: URLSearchParams, remembered: { format?: Format }): { mode: EvalMode; format: Format } {
  const m = params.get('eval_mode');
  const f = Number(params.get('format') ?? NaN);
  const format = ([0, 2, 3, 4] as const).find((x) => x === f) ?? remembered.format ?? 0;
  return { mode: m === 'walk_forward' ? 'walk_forward' : 'standard', format };
}

const AGREEMENT_FAMILY: Record<Family, string> = { glicko2_daneo: 'GlickoD', glicko2_std: 'G2 Std' };

/** agreementName is a model's name on upstream's agreement cards. */
export function agreementName(key: ModelInfo['key']): string {
  const info = MODELS.find((i) => i.key === key);
  if (info === undefined) return key;
  const family = AGREEMENT_FAMILY[info.family];
  if (info.reset === 'softer') return `${family} (Season Reset)`;
  return info.family === 'glicko2_daneo' ? family : `${family} (Career)`;
}

/** parseMoversParams reads the comparison's search and format; upstream ignores its min_games. */
export function parseMoversParams(params: URLSearchParams): { search: string; format: Format } {
  const f = Number(params.get('format') ?? NaN);
  return { search: (params.get('search') ?? '').trim(), format: ([0, 2, 3, 4] as const).find((x) => x === f) ?? 0 };
}

const count = (n: number) => n.toLocaleString('en-US');

/** formatCards returns the activity page's total games and its cards for 3-, 2- and 4-player games, from the run's counts. */
export function formatCards(formats: Activity['formats']) {
  const of = (f: Format) => formats.find((x) => x.format === f) ?? { games: 0, pairs: 0 };
  const card = (format: Format, name: string, note: (pairs: string) => string) => ({
    format,
    name,
    note: note(count(of(format).pairs)),
    games: of(format).games,
  });
  return {
    total: formats.reduce((sum, f) => sum + f.games, 0),
    cards: [
      card(3, '3-Player Games', (p) => `Most popular tournament format (${p} pairwise duels)`),
      card(2, '2-Player (Duels)', (p) => `Pure 1v1 heads-up duels (${p} pairwise games)`),
      card(4, '4-Player Games', (p) => `Full table political strategy (${p} pairwise duels)`),
    ],
  };
}

const FORMAT_EVENT = 'analysis-format';

/** announceFormat tells the analysis page's other parts that the reader chose format. */
export function announceFormat(target: EventTarget, format: Format): void {
  target.dispatchEvent(new CustomEvent<Format>(FORMAT_EVENT, { detail: format }));
}

/** onFormat calls listen with each format announced on target, and returns a function that stops it. */
export function onFormat(target: EventTarget, listen: (format: Format) => void): () => void {
  const handler = (e: Event) => listen((e as CustomEvent<Format>).detail);
  target.addEventListener(FORMAT_EVENT, handler);
  return () => target.removeEventListener(FORMAT_EVENT, handler);
}

/** titleLabel is a title's name on the activity page. */
export function titleLabel(code: TitleCode): string {
  return titleName(code);
}

export type CountryColumn = 'rank' | 'country' | 'players' | 'gm' | 'm_plus' | 'p_plus' | 'all';

/** sortCountries sorts the countries table: numbers with none as 0, and the country code as text. */
export function sortCountries<T extends CountryRatings & { rank: number }>(rows: readonly T[], col: CountryColumn, asc: boolean): T[] {
  const dir = asc ? 1 : -1;
  if (col === 'rank') return [...rows].sort((a, b) => dir * (a.rank - b.rank));
  if (col === 'country') {
    return [...rows].sort((a, b) => dir * String(a[col]).localeCompare(String(b[col])));
  }
  return [...rows].sort((a, b) => dir * ((a[col] ?? 0) - (b[col] ?? 0)));
}

export type SeatTier = 'all' | 'grandmaster' | 'master' | 'advanced' | 'intermediate' | 'beginner';

export interface SeatTierOption {
  value: SeatTier;
  label: string;
  tooltip: string;
}

export const SEAT_TIERS: readonly SeatTierOption[] = [
  { value: 'all', label: 'All Skill Levels', tooltip: 'All matches regardless of rating' },
  { value: 'grandmaster', label: '👑 Grandmaster', tooltip: 'Elite tables (Table Glicko-D avg ≥ 1750)' },
  { value: 'master', label: '🎖️ Master', tooltip: 'Master tables (Table Glicko-D avg 1625–1749)' },
  { value: 'advanced', label: '⚔️ Advanced', tooltip: 'Advanced tables (Table Glicko-D avg 1500–1624)' },
  { value: 'intermediate', label: '🛡️ Intermediate', tooltip: 'Intermediate tables (Table Glicko-D avg 1375–1499)' },
  { value: 'beginner', label: '🌱 Beginner', tooltip: 'Beginner tables (Table Glicko-D avg < 1375)' },
];

/** seatDelta returns the seat advantage delta in percentage points relative to equal probability (100 / playerCount). */
export function seatDelta(winRate: number, playerCount: number): number {
  if (playerCount <= 0) return 0;
  const parity = 100 / playerCount;
  const rounded = Number((winRate - parity).toFixed(2));
  return Object.is(rounded, -0) ? 0 : rounded;
}


/** seatSpread returns the first seat's win rate less the last seat's, in points to two places; null without every seat. */
export function seatSpread(group: SeatStatsGroup | undefined): number | null {
  if (group === undefined || group.seats.length < group.player_count) return null;
  const spread = Number((group.seats[0].win_rate - group.seats[group.player_count - 1].win_rate).toFixed(2));
  return Object.is(spread, -0) ? 0 : spread;
}

/** seatDeltaText formats the delta with explicit sign and two decimal places (e.g. "+2.47%"). */
export function seatDeltaText(delta: number): string {
  return `${signed(delta, 2)}%`;
}

