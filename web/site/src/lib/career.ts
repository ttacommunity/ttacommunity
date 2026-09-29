/** Upstream's career box and head-to-head popup: its year scoring, model notes and shared-game tally. */
import type { CareerYear, Format, Player, PlayerRating, TitleCode, VersusMatch } from '@ttac/shared/ratings';

type Peak = Player['peak'];

/** Upstream's bonus for the best title held in a year. */
const TITLE_BONUS: Record<TitleCode, number> = { WC: 150, GM: 60, M: 40, P: 25, G: 15, S: 5, B: 0, W: -5 };

/** Upstream's ranking of titles, best first. */
const TITLE_ORDER: Record<TitleCode, number> = { WC: 0, GM: 2, M: 3, P: 4, G: 5, S: 6, B: 7, W: 8 };

/** A scored career year, with the title the box shows for it. */
export type ScoredYear = CareerYear & { score: number; title: TitleCode | null };

/**
 * careerPeakYear picks upstream's Peak Career Season: the first year with the highest
 * wins·1.5 + win rate·2 + (peak rating − 1500)·0.4 + the year's title bonus, plus 30 in the career peak's year. A
 * year without a title shows the career peak's. It returns null without years.
 */
export function careerPeakYear(years: readonly CareerYear[], peak: Peak): ScoredYear | null {
  let best: ScoredYear | null = null;
  for (const y of years) {
    const bonus = (y.title === null ? 0 : TITLE_BONUS[y.title]) + (peak?.year === y.year ? 30 : 0);
    const score = y.wins * 1.5 + y.win_rate * 2 + (y.peak_rating ? (y.peak_rating - 1500) * 0.4 : 0) + bonus;
    if (best === null || score > best.score) best = { ...y, score: Math.round(score * 10) / 10 };
  }
  if (best !== null && best.title === null && peak !== null) best.title = peak.code;
  return best;
}

/** divergenceNotes returns upstream's two sentences comparing Standard with MP-Weighted, and WHR with Standard. */
export function divergenceNotes(std?: PlayerRating, mp?: PlayerRating, whr?: PlayerRating): string[] {
  const notes: string[] = [];
  if (std && mp) {
    const diff = std.rating - mp.rating;
    if (diff > 15) {
      notes.push(
        `Rating adjusts downward by ${diff.toFixed(1)} pts under MP-Weighted, reflecting calibration for multiplayer (3p/4p) play where naive dueling over-accumulates confidence.`,
      );
    } else if (diff < -15) {
      notes.push(
        `Rating gains ${Math.abs(diff).toFixed(1)} pts under MP-Weighted, demonstrating high conversion efficiency in complex multiplayer tables.`,
      );
    } else {
      notes.push('Standard and MP-Weighted ratings are tightly aligned, indicating balanced duel and multiplayer tournament exposure.');
    }
  }
  if (std && whr) {
    const diff = whr.rating - std.rating;
    if (diff > 20) {
      notes.push(
        `WHR evaluates career strength ${diff.toFixed(1)} pts higher retrospectively, crediting resilient play across high-variance tournament seasons.`,
      );
    } else if (diff < -20) {
      notes.push(
        `WHR smooths out localized hot streaks by ${Math.abs(diff).toFixed(1)} pts, grounding historical trajectory against opponents' lifetime records.`,
      );
    } else {
      notes.push('WHR retrospective reconstruction confirms the forward Glicko-2 trajectory with exceptional fidelity.');
    }
  }
  return notes;
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
