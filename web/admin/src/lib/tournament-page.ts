/** A tournament's settings page in admin: its address, and the settings it edits. */

export type Setting = 'visible' | 'rated' | 'community';

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

/** toggledCounts turns a player count's rating on or off, or gives null where that would leave none rated. */
export function toggledCounts(counts: readonly number[], n: number, on: boolean): number[] | null {
  const next = on ? [...new Set([...counts, n])] : counts.filter((c) => c !== n);
  return next.length === 0 ? null : next.sort((a, b) => a - b);
}

/** divisionList reads a league's top divisions from a comma-separated box, dropping blanks. */
export function divisionList(text: string): string[] {
  return text
    .split(',')
    .map((d) => d.trim())
    .filter((d) => d !== '');
}

/** divisionText writes top divisions for the box divisionList reads. */
export function divisionText(divisions: readonly string[]): string {
  return divisions.join(', ');
}

/** renamedFrom returns the name something came in with, or null when it has not been renamed. */
export function renamedFrom(name: string, original: string): string | null {
  return original !== '' && original !== name ? original : null;
}
