/** A tournament's settings page in admin: its address, and the settings it edits. */
import type { TournamentSettings } from './admin-api';

export type Setting = keyof Required<TournamentSettings>;

/** SETTINGS are a tournament's on/off settings, in the order admin shows them. */
export const SETTINGS: { key: Setting; label: string; short: string; hint: string }[] = [
  { key: 'visible', label: 'Shown', short: 'Shown', hint: 'Shown on the site even when not rated' },
  { key: 'rated', label: 'Rated', short: 'Rated', hint: 'In the community ratings, from the next engine run' },
  { key: 'community', label: 'Community leaderboard', short: 'Community', hint: 'Counted in the overall community leaderboard' },
];

// The API's pattern for a tournament in a path.
const SLUG = /^[a-z][a-z0-9_]{0,63}$/;

/** settingsHref is the address of a tournament's settings page. */
export function settingsHref(slug: string): string {
  return `/tournament/?t=${encodeURIComponent(slug)}`;
}

/** slugParam returns the tournament the address names (?t=), or null for none or one the API would refuse. */
export function slugParam(search: URLSearchParams): string | null {
  const t = search.get('t');
  return t !== null && SLUG.test(t) ? t : null;
}
