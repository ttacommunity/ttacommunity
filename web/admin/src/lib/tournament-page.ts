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

/** settingsHref is the address of a tournament's or group's page; with settings, a group's opens on its settings. */
export function settingsHref(slug: string, settings = false): string {
  return `/tournament/?t=${encodeURIComponent(slug)}${settings ? '&open=settings' : ''}`;
}

/** opensSettings says whether the address asks for a group's settings open. */
export function opensSettings(search: URLSearchParams): boolean {
  return search.get('open') === 'settings';
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

/**
 * pointsList reads points per place for a game of that many players ("6-3-1-0", commas or spaces too):
 * [] for an empty box, or null where the API would refuse them (not one per place, negative, or higher for a worse place).
 */
export function pointsList(text: string, players: number): number[] | null {
  const parts = text.trim() === '' ? [] : text.trim().split(/\s*[-,\s]\s*/);
  if (parts.length === 0) return [];
  const points = parts.map((p) => (/^\d+(\.\d+)?$/.test(p) ? Number(p) : NaN));
  if (points.length !== players || points.some((p, i) => Number.isNaN(p) || (i > 0 && p > points[i - 1]))) return null;
  return points;
}

/** pointsText writes points per place for the box pointsList reads. */
export function pointsText(points: readonly number[]): string {
  return points.join('-');
}

type Scoring = { points_2p: number[]; points_3p: number[]; points_4p: number[] };

/**
 * scoringPatch returns the points per place to send for the boxes (by player count) that differ from saved,
 * {} when none do, or null when a box holds points the API would refuse.
 */
export function scoringPatch(boxes: Record<2 | 3 | 4, string>, saved: Scoring): Partial<Scoring> | null {
  const patch: Partial<Scoring> = {};
  for (const n of [2, 3, 4] as const) {
    const points = pointsList(boxes[n], n);
    if (points === null) return null;
    const key = `points_${n}p` as const;
    if (pointsText(points) !== pointsText(saved[key])) patch[key] = points;
  }
  return patch;
}

/** hasPenalty reports whether a season can have a time-out penalty: a league's, numbered. */
export function hasPenalty(kind: string, season: string): boolean {
  return kind === 'league' && /^\d+$/.test(season);
}

/** penaltyValue reads a season's time-out penalty, 0 or below, or null for anything else. */
export function penaltyValue(text: string): number | null {
  const t = text.trim();
  if (!/^-?\d+(\.\d+)?$/.test(t)) return null;
  const n = Number(t);
  return n > 0 ? null : n;
}

/** renamedFrom returns the name something came in with, or null when it has not been renamed. */
export function renamedFrom(name: string, original: string): string | null {
  return original !== '' && original !== name ? original : null;
}

type LevelNames = { season_name: string; stage_name: string };

/**
 * levelNamesPatch returns the season and stage names to send, trimmed, for those that differ from saved ('' is the default),
 * {} when none do, or null when one is longer than the API allows (32 characters).
 */
export function levelNamesPatch(boxes: LevelNames, saved: LevelNames): Partial<LevelNames> | null {
  const patch: Partial<LevelNames> = {};
  for (const key of ['season_name', 'stage_name'] as const) {
    const name = boxes[key].trim();
    if ([...name].length > 32) return null;
    if (name !== saved[key]) patch[key] = name;
  }
  return patch;
}
