/** Aggregation and transformation functions for the landing page community pulse feed. */
import {
  catalogueGames,
  catalogueTournament,
  playerMatches,
  RatingsApiError,
  type CatalogueGame,
  type PlayerMatch,
} from '@ttac/shared/ratings';
import type { FeedSettings, TournamentFeedFilter } from './feed-settings';

export interface FeedMatchSeat {
  name: string;
  score: number;
  outcome?: string | null;
}

export interface FeedMatch {
  matchId: number;
  tournament: string;
  division?: string;
  date: string;
  playerCount: number;
  replayCode: string | null;
  seats: FeedMatchSeat[];
}

/** Transforms a CatalogueGame from a tournament query into a FeedMatch. */
export function catalogueGameToFeedMatch(g: CatalogueGame, tournamentName: string): FeedMatch {
  return {
    matchId: g.match_id,
    tournament: tournamentName,
    division: g.group || undefined,
    date: g.date,
    playerCount: g.seats.length,
    replayCode: g.replay_code,
    seats: g.seats.map((s) => ({
      name: s.name,
      score: s.score,
      outcome: s.outcome,
    })),
  };
}

/** Transforms a PlayerMatch from an individual player query into a FeedMatch. */
export function playerMatchToFeedMatch(m: PlayerMatch): FeedMatch {
  return {
    matchId: m.match_id,
    tournament: m.tournament,
    date: m.date,
    playerCount: m.player_count,
    replayCode: m.replay_code,
    seats: m.seats.map((s) => ({
      name: s.name,
      score: s.score,
    })),
  };
}


/** Merges matches, deduplicates by matchId (preserving richer division data), and sorts descending by date. */
export function mergeFeedMatches(matches: FeedMatch[]): FeedMatch[] {
  const map = new Map<number, FeedMatch>();
  for (const m of matches) {
    const existing = map.get(m.matchId);
    if (!existing) {
      map.set(m.matchId, m);
    } else if (!existing.division && m.division) {
      // Retain the version that specifies the division
      map.set(m.matchId, { ...existing, division: m.division });
    }
  }

  return Array.from(map.values()).sort((a, b) => {
    const cmp = b.date.localeCompare(a.date);
    return cmp !== 0 ? cmp : b.matchId - a.matchId;
  });
}

export const PREMIER_DIVISIONS: Record<string, string[]> = {
  royal_league: ['Emperor', 'King 1', 'King 2', 'Prince 1', 'Prince 2', 'Duke 1', 'Marquess 1', 'Count 1'],
  international: ['Grandmaster', 'Master', 'Diamond', 'Platinum', 'Gold', 'Silver', 'Bronze'],
  intermezzo: ['Grandmaster', 'Diamond', 'Platinum', 'Gold', 'Silver', 'Bronze'],
};

/** divisionsToFetch returns the tournaments whose divisions are still to be asked for: each slug once, in order, none in asked. */
export function divisionsToFetch(tournaments: readonly TournamentFeedFilter[], asked: ReadonlySet<string>): string[] {
  return [...new Set(tournaments.map((t) => t.slug))].filter((slug) => !asked.has(slug));
}

/** fetchTournamentDivisions returns the groups of a tournament's latest season, none when it has no seasons; it rejects when the API fails. */
export async function fetchTournamentDivisions(apiUrl: string, runId: number, slug: string, fetchFn: typeof fetch = fetch): Promise<string[]> {
  const latest = (await catalogueTournament(apiUrl, runId, slug, fetchFn)).seasons.at(-1);
  if (latest === undefined) return [];
  return (await catalogueGames(apiUrl, runId, slug, { season: latest.label, page: 1 }, fetchFn)).groups;
}

/** isFinishedCatalogueGame checks that a game is completed (state is 'played' and has at least two seats). */
export function isFinishedCatalogueGame(g: CatalogueGame): boolean {
  return g.state === 'played' && Array.isArray(g.seats) && g.seats.length >= 2;
}

/** skipMissing gives no games for a source the API does not know, such as a saved player since gone, and passes any other failure on. */
function skipMissing(e: unknown): FeedMatch[] {
  if (e instanceof RatingsApiError && e.status === 404) return [];
  throw e;
}

/** Fetches and combines games for all configured tournaments/divisions and players; a source that fails, other than
 * one the API does not know, fails it. */
export async function fetchFeedMatches(
  apiUrl: string,
  runId: number,
  settings: FeedSettings,
  limit = 8,
  fetchFn: typeof fetch = fetch,
): Promise<FeedMatch[]> {
  const tourneyPromises = settings.tournaments.map((t) =>
    catalogueGames(apiUrl, runId, t.slug, { group: t.division || undefined, state: 'played', page: 1 }, fetchFn)
      .then((res) => res.games.filter(isFinishedCatalogueGame).map((g) => catalogueGameToFeedMatch(g, t.name)))
      .catch(skipMissing),
  );

  const playerPromises = settings.players.slice(0, 12).map((player) =>
    playerMatches(apiUrl, runId, player, 0, 'glicko2_daneo', 1, fetchFn)
      .then((res) => res.matches.map(playerMatchToFeedMatch))
      .catch(skipMissing),
  );

  const results = await Promise.all([...tourneyPromises, ...playerPromises]);
  const flat = results.flat();
  const sorted = mergeFeedMatches(flat);
  return sorted.slice(0, limit);
}

