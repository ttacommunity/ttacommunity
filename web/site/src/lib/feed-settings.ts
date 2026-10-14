/** Game feed user configuration and persistence. */

export interface TournamentFeedFilter {
  slug: string;
  name: string;
  /** Specific division / group filter (e.g. "Grandmaster", "Emperor"), or empty string for all divisions. */
  division?: string;
}

export interface FeedSettings {
  tournaments: TournamentFeedFilter[];
  players: string[];
}

/** The default premier circuits and their top divisions. */
export const PREMIER_TOURNAMENTS: TournamentFeedFilter[] = [
  { slug: 'international', name: 'International Championship', division: 'Grandmaster' },
  { slug: 'intermezzo', name: 'Intermezzo Championship', division: 'Grandmaster' },
  { slug: 'royal_league', name: 'Royal League', division: 'Emperor' },
];

export const DEFAULT_FEED_SETTINGS: FeedSettings = {
  tournaments: PREMIER_TOURNAMENTS,
  players: [],
};

const STORAGE_KEY = 'ttac.gamefeed_filter';

/** Sanitizes untrusted or parsed JSON into a valid FeedSettings object. */
export function sanitizeFeedSettings(data: unknown): FeedSettings {
  if (!data || typeof data !== 'object') {
    return DEFAULT_FEED_SETTINGS;
  }
  const obj = data as Record<string, unknown>;
  const rawTournaments = Array.isArray(obj.tournaments) ? obj.tournaments : [];
  const rawPlayers = Array.isArray(obj.players) ? obj.players : [];

  const tournaments: TournamentFeedFilter[] = [];
  for (const t of rawTournaments) {
    if (t && typeof t === 'object' && typeof (t as { slug?: unknown }).slug === 'string') {
      const item = t as Record<string, unknown>;
      tournaments.push({
        slug: String(item.slug),
        name: typeof item.name === 'string' ? item.name : String(item.slug),
        division: typeof item.division === 'string' ? item.division : '',
      });
    }
  }

  const players: string[] = [];
  for (const p of rawPlayers) {
    if (typeof p === 'string' && p.trim()) {
      players.push(p.trim());
    }
  }

  if (tournaments.length === 0 && players.length === 0) {
    return DEFAULT_FEED_SETTINGS;
  }

  return { tournaments, players };
}

/** Loads saved feed preferences or returns the default premier settings. */
export function loadFeedSettings(store: Storage): FeedSettings {
  try {
    const raw = store.getItem(STORAGE_KEY);
    if (!raw) return DEFAULT_FEED_SETTINGS;
    return sanitizeFeedSettings(JSON.parse(raw));
  } catch {
    return DEFAULT_FEED_SETTINGS;
  }
}

/** Saves feed preferences; storage failures (e.g. private window) are ignored safely. */
export function saveFeedSettings(store: Storage, settings: FeedSettings): void {
  try {
    store.setItem(STORAGE_KEY, JSON.stringify(settings));
  } catch {
    // Private windows and blocked storage refuse; page continues without memory.
  }
}
