import { describe, expect, it } from 'vitest';
import type { LeaderboardRow } from '@ttac/shared/ratings';
import { defaultView, INACTIVE_ON_OR_BEFORE, MIN_OPPONENTS } from './leaderboard-view';

function row(name: string, c_rating: number, opponents: number, last_played: string | null, rank: number | null = 1): LeaderboardRow {
  return {
    rank, rank_delta: 0, name, rating: c_rating + 100, rd: 30, c_rating, games: opponents, opponents,
    wins: 0, losses: 0, draws: 0, win_rate: 0, last_played, country: null, title: null,
  };
}

describe('defaultView', () => {
  it('uses upstream defaults', () => {
    expect(INACTIVE_ON_OR_BEFORE).toBe('2025-05-31');
    expect(MIN_OPPONENTS).toBe(30);
  });

  it('keeps only active players with enough opponents', () => {
    const got = defaultView([
      row('active', 1800, 30, '2025-06-01'),
      row('cutoff-day', 1900, 50, '2025-05-31'),
      row('never', 1950, 50, null),
      row('too-few', 1990, 29, '2026-01-01'),
    ]);
    expect(got.map((p) => p.name)).toEqual(['active']);
  });

  it('ranks by conservative rating from 1, ignoring the stored rank', () => {
    const got = defaultView([
      row('third', 1700, 40, '2026-01-01', 2),
      row('first', 1900, 40, '2026-01-01', 9),
      row('second', 1800, 40, '2026-01-01', null),
    ]);
    expect(got.map((p) => [p.displayRank, p.name])).toEqual([
      [1, 'first'],
      [2, 'second'],
      [3, 'third'],
    ]);
  });

  it('does not change the rows it was given', () => {
    const rows = [row('b', 1700, 40, '2026-01-01'), row('a', 1900, 40, '2026-01-01')];
    defaultView(rows);
    expect(rows.map((p) => p.name)).toEqual(['b', 'a']);
  });
});
