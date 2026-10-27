/** The front page's decisions: how a game's seats are coloured. */

/**
 * seatRankClass colours a revealed seat by its score among the game's: the winner gold, then silver and bronze, and
 * the rest gray (in a two-player game the loser is gray); everyone gold when all scores are equal.
 */
export function seatRankClass(score: number, scores: readonly number[]): string {
  if (scores.length < 2) return 'rank-gold';
  const sorted = [...scores].sort((a, b) => b - a);
  const best = sorted[0];
  const worst = sorted[sorted.length - 1];
  if (best === worst || score === best) return 'rank-gold';
  if (scores.length === 2) return 'rank-gray';
  const rank = sorted.indexOf(score) + 1;
  if (scores.length === 3) return rank === 2 ? 'rank-silver' : 'rank-bronze';
  if (score === worst) return 'rank-gray';
  if (rank === 2) return 'rank-silver';
  if (rank === 3) return 'rank-bronze';
  return 'rank-gray';
}
