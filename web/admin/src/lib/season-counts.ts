/** A season's stages as admin lists them, with their finished games and players, and the season's summary line. */
import type { components } from '@ttac/shared/api';

export type SeasonCount = components['schemas']['SeasonCount'];
type Stage = { label: string; original_name: string; note: string };

/**
 * stageRows returns the stages with finished games, the highest number first (by name where numbers tie), each with
 * its finished games and its players from count.
 */
export function stageRows<T extends Stage>(stages: readonly T[], count: SeasonCount | undefined): (T & { games: number; players: number })[] {
  return stages
    .flatMap((st) => {
      const c = count?.stages.find((x) => x.stage === st.label);
      return c === undefined || c.finished === 0 ? [] : [{ st, c }];
    })
    .sort((a, b) => b.c.number - a.c.number || b.st.label.localeCompare(a.st.label, undefined, { numeric: true }))
    .map(({ st, c }) => ({ ...st, games: c.finished, players: c.players }));
}

/** seasonSummary returns a season's stages with finished games, their finished games, and its players. */
export function seasonSummary(stages: readonly Stage[], count: SeasonCount | undefined): { stages: number; games: number; players: number } {
  const rows = stageRows(stages, count);
  return { stages: rows.length, games: rows.reduce((n, r) => n + r.games, 0), players: count?.players ?? 0 };
}
