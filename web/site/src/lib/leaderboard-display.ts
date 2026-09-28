/** How the leaderboard shows models, formats and numbers, in upstream's wording and precision. */
import type { Format, Model, TitleCode } from '@ttac/shared/ratings';

const formatNames: Record<Format, string> = { 0: 'All Formats', 2: '2-Player (Duel)', 3: '3-Player', 4: '4-Player' };

const engineNames: Record<string, string> = {
  glicko2_daneo: 'GlickoD',
  glicko2_std: 'Glicko-2 Standard',
  glicko2_mp: 'Glicko-2 MP-Weighted',
  glicko2_adapt: 'Glicko-2 Adaptive-T',
  whr: 'Whole-History Rating',
};

/** formatName returns upstream's name for a format. */
export function formatName(format: Format): string {
  return formatNames[format];
}

/** modelLabel names a model by its engine, then its reset mode and prior when they are not the defaults. */
export function modelLabel(model: Model): string {
  const engine = model.replace(/(_softer)?(_retro)?$/, '');
  const parts = [engineNames[engine] ?? engine];
  if (model.includes('_softer')) parts.push('Season Reset');
  if (model.endsWith('_retro')) parts.push('15G Retro');
  return parts.join(' · ');
}

/** rank returns "#n", or a dash for an unranked player. */
export function rank(n: number | null): string {
  return n === null ? '–' : `#${n}`;
}

/** isTopThree reports whether a rank gets upstream's gold highlight. */
export function isTopThree(n: number | null): boolean {
  return n !== null && n <= 3;
}

/** cRating shows a conservative rating to two places. */
export function cRating(value: number): string {
  return value.toFixed(2);
}

/** rating shows a rating to one place. */
export function rating(value: number): string {
  return value.toFixed(1);
}

/** rd shows a rating deviation as "±n.n". */
export function rd(value: number): string {
  return `±${value.toFixed(1)}`;
}

/** winRate shows a percentage to one place. */
export function winRate(value: number): string {
  return `${value.toFixed(1)}%`;
}

const titleNames: Record<TitleCode, string> = {
  WC: 'World Champion',
  GM: 'Grandmaster',
  M: 'Master',
  P: 'Platinum',
  G: 'Gold',
  S: 'Silver',
  B: 'Bronze',
  W: 'Wood',
};

/** titleName returns a title's full name, shown when the run gave no reason. */
export function titleName(code: TitleCode): string {
  return titleNames[code];
}

/** flagSrc returns the path of a country's flag image. */
export function flagSrc(country: string): string {
  return `/flags/${country.toLowerCase()}.svg`;
}
