/** Upstream's default leaderboard view: active players with enough opponents, ranked among themselves. */
import type { LeaderboardRow } from '@ttac/shared/ratings';

/** A player whose last game is on or before this date is inactive. Upstream's fixed date, not yet a rolling rule. */
export const INACTIVE_ON_OR_BEFORE = '2025-05-31';

/** The fewest opponents a player needs to appear by default. */
export const MIN_OPPONENTS = 30;

export type RankedRow = LeaderboardRow & { displayRank: number };

/** defaultView keeps active players with at least MIN_OPPONENTS, ranked 1… by conservative rating. */
export function defaultView(rows: readonly LeaderboardRow[]): RankedRow[] {
  return rows
    .filter((p) => p.opponents >= MIN_OPPONENTS && p.last_played !== null && p.last_played > INACTIVE_ON_OR_BEFORE)
    .sort((a, b) => b.c_rating - a.c_rating || a.name.localeCompare(b.name))
    .map((p, i) => ({ ...p, displayRank: i + 1 }));
}
