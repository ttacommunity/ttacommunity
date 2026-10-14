import { describe, expect, it } from 'vitest';
import {
  catalogueGameToFeedMatch,
  playerMatchToFeedMatch,
  mergeFeedMatches,
  type FeedMatch,
} from './feed-data';
import type { CatalogueGame, PlayerMatch } from '@ttac/shared/ratings';

describe('feed-data', () => {
  it('converts a CatalogueGame into a FeedMatch', () => {
    const cg: CatalogueGame = {
      match_id: 101,
      date: '2026-09-15',
      season: 'Season 9',
      stage: '',
      group: 'Emperor',
      game: 1,
      state: 'played',
      replay_code: 'abc1234',
      rated: true,
      seats: [
        { name: 'Alice', score: 210, outcome: 'finished' },
        { name: 'Bob', score: 195, outcome: 'finished' },
      ],
    };

    const feed = catalogueGameToFeedMatch(cg, 'Royal League');
    expect(feed).toEqual({
      matchId: 101,
      tournament: 'Royal League',
      division: 'Emperor',
      date: '2026-09-15',
      playerCount: 2,
      replayCode: 'abc1234',
      seats: [
        { name: 'Alice', score: 210, outcome: 'finished' },
        { name: 'Bob', score: 195, outcome: 'finished' },
      ],
    });
  });

  it('converts a PlayerMatch into a FeedMatch', () => {
    const pm: PlayerMatch = {
      match_id: 102,
      tournament: 'International',
      date: '2026-09-10',
      player_count: 4,
      placement: 1,
      replay_code: null,
      rating_delta: 12.5,
      seats: [
        { name: 'Alice', score: 250 },
        { name: 'Bob', score: 220 },
      ],

    };

    const feed = playerMatchToFeedMatch(pm);
    expect(feed).toEqual({
      matchId: 102,
      tournament: 'International',
      division: undefined,
      date: '2026-09-10',
      playerCount: 4,
      replayCode: null,
      seats: [
        { name: 'Alice', score: 250 },
        { name: 'Bob', score: 220 },
      ],
    });
  });


  it('merges, deduplicates by matchId, and sorts descending by date', () => {
    const m1: FeedMatch = {
      matchId: 1,
      tournament: 'RL',
      date: '2026-09-01',
      playerCount: 2,
      replayCode: null,
      seats: [],
    };
    const m2: FeedMatch = {
      matchId: 2,
      tournament: 'IC',
      date: '2026-09-05',
      playerCount: 4,
      replayCode: null,
      seats: [],
    };
    const m3Duplicate: FeedMatch = {
      matchId: 2,
      tournament: 'IC',
      division: 'Grandmaster',
      date: '2026-09-05',
      playerCount: 4,
      replayCode: null,
      seats: [],
    };
    const m4: FeedMatch = {
      matchId: 4,
      tournament: 'IM',
      date: '2026-09-10',
      playerCount: 3,
      replayCode: null,
      seats: [],
    };

    const merged = mergeFeedMatches([m1, m2, m3Duplicate, m4]);
    expect(merged.map((m) => m.matchId)).toEqual([4, 2, 1]);
    // Duplicate keeps the version with richer metadata (e.g. division)
    expect(merged[1]?.division).toBe('Grandmaster');
  });

  it('provides sensible fallback divisions for premier tournaments', async () => {
    const { PREMIER_DIVISIONS } = await import('./feed-data');
    expect(PREMIER_DIVISIONS.royal_league).toContain('Emperor');
    expect(PREMIER_DIVISIONS.international).toContain('Grandmaster');
    expect(PREMIER_DIVISIONS.intermezzo).toContain('Grandmaster');
  });
});


