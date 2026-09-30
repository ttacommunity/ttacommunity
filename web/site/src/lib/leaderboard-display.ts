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

/** engineName returns upstream's name for a model's engine, without its reset mode or prior. */
export function engineName(model: Model): string {
  const engine = model.replace(/(_softer)?(_retro)?$/, '');
  return engineNames[engine] ?? engine;
}

const engineNotes: Record<string, string> = {
  glicko2_std: 'Standard 1v1 naive baseline',
  glicko2_mp: 'Multiplayer fractional weighting — variance-calibrated',
  glicko2_adapt: 'Adaptive temperature scaling',
  whr: 'Whole-History Rating retrospective MAP',
};

/** engineNote returns upstream's short description of a model's engine, or '' for GlickoD. */
export function engineNote(model: Model): string {
  return engineNotes[model.replace(/(_softer)?(_retro)?$/, '')] ?? '';
}

/** resetModeName returns upstream's name for a model's season reset mode. */
export function resetModeName(model: Model): string {
  return model.includes('_softer') ? 'Season Reset' : 'Continuous (Career)';
}

/** modelLabel names a model by its engine, then its reset mode and prior when they are not the defaults. */
export function modelLabel(model: Model): string {
  const parts = [engineName(model)];
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
  SGM: 'Super GM',
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

/** delta shows a rating change as upstream: an arrow and one decimal, or a dash within ±0.05. */
export function delta(value: number): { text: string; kind: 'delta-pos' | 'delta-neg' | 'delta-zero' } {
  if (value > 0.05) return { text: `↑+${value.toFixed(1)}`, kind: 'delta-pos' };
  if (value < -0.05) return { text: `↓${value.toFixed(1)}`, kind: 'delta-neg' };
  return { text: '—0.0', kind: 'delta-zero' };
}
