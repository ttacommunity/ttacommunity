/** The front page's decisions: what each of its requests became, and how a game's seats are coloured. */

/** A part of the page's data: loading, could not load (with why), or ready. */
export type Section<T> = { kind: 'loading' } | { kind: 'error'; message: string } | { kind: 'ready'; data: T };

/** settled waits for a request and gives its answer as ready, or its failure as an error; it never rejects. */
export async function settled<T>(request: Promise<T>): Promise<Section<T>> {
  try {
    return { kind: 'ready', data: await request };
  } catch (e: unknown) {
    return { kind: 'error', message: e instanceof Error ? e.message : String(e) };
  }
}

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
