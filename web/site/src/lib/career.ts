/** Upstream's career box and head-to-head popup: its year scoring and shared-game tally. */
import type { CareerYear, Format, Player, TitleCode, VersusMatch } from '@ttac/shared/ratings';

type Peak = Player['peak'];

/** Upstream's bonus for the best title held in a year. */
const TITLE_BONUS: Record<TitleCode, number> = { WC: 150, SGM: 100, GM: 60, M: 40, P: 25, G: 15, S: 5, B: 0, W: -5 };

/** Upstream's ranking of titles, best first. */
const TITLE_ORDER: Record<TitleCode, number> = { WC: 0, SGM: 1, GM: 2, M: 3, P: 4, G: 5, S: 6, B: 7, W: 8 };

/** A scored career year, with the title the box shows for it. */
export type ScoredYear = CareerYear & { score: number; title: TitleCode | null };

/**
 * scoreYears scores each year as upstream: wins·1.5 + win rate·2 + (peak rating − 1500)·0.4 + the year's title
 * bonus, plus 30 in the career peak's year, rounded to 0.1.
 */
export function scoreYears(years: readonly CareerYear[], peak: Peak): ScoredYear[] {
  return years.map((y) => ({ ...y, score: Math.round(rawScore(y, peak) * 10) / 10 }));
}

function rawScore(y: CareerYear, peak: Peak): number {
  const bonus = (y.title === null ? 0 : TITLE_BONUS[y.title]) + (peak?.year === y.year ? 30 : 0);
  return y.wins * 1.5 + y.win_rate * 2 + (y.peak_rating ? (y.peak_rating - 1500) * 0.4 : 0) + bonus;
}

/**
 * careerPeakYear picks upstream's Peak Career Season: the first year with the highest score. Without its own title
 * it shows the career peak's. It returns null without years.
 */
export function careerPeakYear(years: readonly CareerYear[], peak: Peak): ScoredYear | null {
  // Upstream compares the unrounded scores.
  const scored = scoreYears(years, peak);
  let best: ScoredYear | null = null;
  let bestRaw = -Infinity;
  for (let i = 0; i < scored.length; i++) {
    const raw = rawScore(years[i], peak);
    if (raw > bestRaw) {
      best = scored[i];
      bestRaw = raw;
    }
  }
  if (best !== null && best.title === null && peak !== null) best = { ...best, title: peak.code };
  return best;
}

/** wcLabel is upstream's header badge for a World Champion: the reigning one, else the year of a past title. */
export function wcLabel(title: Player['title'], peak: Peak): string | null {
  if (title?.code === 'WC') return 'Reigning WC';
  if (peak?.code === 'WC' && peak.year !== null) return `${peak.year} WC`;
  return null;
}

/** isPeakHigher reports whether the career peak, with its year, outranks the current title, as upstream decides it. */
export function isPeakHigher(title: TitleCode | null, peak: Peak): boolean {
  if (peak === null || peak.year === null) return false;
  return title === null || TITLE_ORDER[peak.code] < TITLE_ORDER[title];
}

/** versusTally counts the shared games in a format (0 is all) and who finished ahead in each. */
export function versusTally(
  matches: readonly VersusMatch[],
  format: Format,
): { games: number; nameWins: number; otherWins: number; draws: number; rows: VersusMatch[] } {
  const rows = format === 0 ? [...matches] : matches.filter((m) => m.player_count === format);
  const nameWins = rows.filter((m) => m.name_place < m.other_place).length;
  const otherWins = rows.filter((m) => m.other_place < m.name_place).length;
  return { games: rows.length, nameWins, otherWins, draws: rows.length - nameWins - otherWins, rows };
}
