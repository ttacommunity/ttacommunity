/** The front page's decisions: where a game's seats finished, and which games and podiums are revealed. */
import type { Place } from './player-link';

/**
 * seatPlace places a revealed seat by its score among the game's: the winner first, then second and third, and the
 * rest last (in a two-player game the loser is last; of three, third); everyone first when all scores are equal.
 */
export function seatPlace(score: number, scores: readonly number[]): Place {
  if (scores.length < 2) return 1;
  const sorted = [...scores].sort((a, b) => b - a);
  const best = sorted[0];
  const worst = sorted[sorted.length - 1];
  if (best === worst || score === best) return 1;
  if (scores.length === 2) return 'last';
  const rank = sorted.indexOf(score) + 1;
  if (scores.length === 3) return rank === 2 ? 2 : 3;
  if (score === worst) return 'last';
  if (rank === 2) return 2;
  if (rank === 3) return 3;
  return 'last';
}
