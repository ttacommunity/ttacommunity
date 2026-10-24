import { describe, expect, it } from 'vitest';
import {
  catalogueGameToFeedMatch,
  divisionsToFetch,
  fetchFeedMatches,
  fetchTournamentDivisions,
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

  it('knows the premier tournaments\' divisions before asking', async () => {
    const { PREMIER_DIVISIONS } = await import('./feed-data');
    expect(PREMIER_DIVISIONS.royal_league).toContain('Emperor');
    expect(PREMIER_DIVISIONS.international).toContain('Grandmaster');
    expect(PREMIER_DIVISIONS.intermezzo).toContain('Grandmaster');
  });
});

describe('fetchFeedMatches', () => {
  const game = { match_id: 7, tournament: 'Cup', date: '2026-09-10', player_count: 2, placement: 1, replay_code: null, rating_delta: null, seats: [{ name: 'ann', score: 9 }, { name: 'bob', score: 5 }] };
  const answering = (byPlayer: Record<string, number>) =>
    (async (input: RequestInfo | URL) => {
      const player = decodeURIComponent(/players\/([^/]+)\/matches/.exec(String(input))?.[1] ?? '');
      const status = byPlayer[player] ?? 500;
      const body = status === 200 ? { name: player, model: 'glicko2_daneo', page: 1, pages: 1, total: 1, matches: [game] } : { error: 'no' };
      return new Response(JSON.stringify(body), { status });
    }) as typeof fetch;

  it('leaves out a saved player the API no longer knows', async () => {
    const got = await fetchFeedMatches('https://api.example', 5, { tournaments: [], players: ['gone', 'ann'] }, 8, answering({ gone: 404, ann: 200 }));
    expect(got.map((m) => m.matchId)).toEqual([7]);
  });

  it('fails when a source cannot be read, rather than show fewer games', async () => {
    const got = fetchFeedMatches('https://api.example', 5, { tournaments: [], players: ['ann', 'bob'] }, 8, answering({ ann: 200, bob: 500 }));
    await expect(got).rejects.toMatchObject({ status: 500 });
  });

  it('only includes finished played games from tournaments', async () => {
    const mockFetch = (async (input: RequestInfo | URL) => {
      const url = String(input);
      expect(url).toContain('state=played');
      const body = {
        total: 3,
        page: 1,
        pages: 1,
        groups: ['Emperor'],
        games: [
          {
            match_id: 201,
            date: '2026-10-01',
            season: 'Season 10',
            stage: '',
            group: 'Emperor',
            game: 1,
            state: 'played',
            replay_code: null,
            rated: true,
            seats: [{ name: 'ann', score: 100 }, { name: 'bob', score: 80 }],
          },
          {
            match_id: 202,
            date: '2026-10-02',
            season: 'Season 10',
            stage: '',
            group: 'Emperor',
            game: 2,
            state: 'running',
            replay_code: null,
            rated: false,
            seats: [{ name: 'ann', score: 0 }, { name: 'bob', score: 0 }],
          },
          {
            match_id: 203,
            date: '2026-10-03',
            season: 'Season 10',
            stage: '',
            group: 'Emperor',
            game: 3,
            state: 'played',
            replay_code: null,
            rated: true,
            seats: [{ name: 'cat', score: 50 }],
          },
        ],
      };
      return new Response(JSON.stringify(body), { status: 200 });
    }) as typeof fetch;

    const got = await fetchFeedMatches(
      'https://api.example',
      5,
      { tournaments: [{ slug: 'royal_league', name: 'Royal League', division: 'Emperor' }], players: [] },
      8,
      mockFetch,
    );
    expect(got.map((m) => m.matchId)).toEqual([201]);
  });
});

describe('divisionsToFetch', () => {
  it.each([
    ['each tournament once, in order', ['royal', 'cup', 'royal'], [], ['royal', 'cup']],
    ['none already asked for', ['royal', 'cup'], ['royal'], ['cup']],
    ['nothing when every one was asked for', ['cup', 'cup'], ['cup'], []],
  ])('gives %s', (_name, slugs, asked, want) => {
    expect(divisionsToFetch(slugs.map((slug) => ({ slug, name: slug })), new Set(asked))).toEqual(want);
  });
});

describe('fetchTournamentDivisions', () => {
  const seasons = (labels: string[]) => labels.map((label) => ({ label }));
  /** api answers the tournament with its seasons, and each season's games with groups; it records every URL asked. */
  const api = (labels: string[], groups: Record<string, string[]>, failGames = false) => {
    const asked: string[] = [];
    const fetchFn = (async (input: RequestInfo | URL) => {
      const url = String(input);
      asked.push(url);
      if (!url.includes('/games?')) return Response.json({ slug: 'cup', seasons: seasons(labels) });
      if (failGames) return Response.json({ error: 'no' }, { status: 500 });
      const season = new URL(url).searchParams.get('season') ?? '';
      return Response.json({ groups: groups[season] ?? [], games: [] });
    }) as typeof fetch;
    return { asked, fetchFn };
  };

  it("gives the latest season's groups, asking for that season alone", async () => {
    const { asked, fetchFn } = api(['1', '2', '3'], { '1': ['Old'], '3': ['A', 'B'] });
    expect(await fetchTournamentDivisions('https://api.example', 5, 'cup', fetchFn)).toEqual(['A', 'B']);
    expect(asked.filter((u) => u.includes('/games?'))).toHaveLength(1);
  });

  it('gives no divisions when the latest season has no groups, without walking older seasons', async () => {
    const { asked, fetchFn } = api(['1', '2'], { '1': ['Old'] });
    expect(await fetchTournamentDivisions('https://api.example', 5, 'cup', fetchFn)).toEqual([]);
    expect(asked.filter((u) => u.includes('/games?'))).toHaveLength(1);
  });

  it('gives no divisions for a tournament with no seasons, asking for no games', async () => {
    const { asked, fetchFn } = api([], {});
    expect(await fetchTournamentDivisions('https://api.example', 5, 'cup', fetchFn)).toEqual([]);
    expect(asked).toHaveLength(1);
  });

  it('fails when the API fails, rather than give a fixed list', async () => {
    const { fetchFn } = api(['1'], {}, true);
    await expect(fetchTournamentDivisions('https://api.example', 5, 'royal_league', fetchFn)).rejects.toMatchObject({ status: 500 });
  });
});
