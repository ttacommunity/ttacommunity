/** A tournament's settings page in admin: its address, and the settings it edits. */
import type { Load } from '@ttac/shared/load';
import { isSlug } from '@ttac/shared/ratings';
import type { AdminResult, AdminTournament, AdminTournamentRow } from './admin-api';

export type Setting = 'visible' | 'rated' | 'community' | 'blitz';

/** SETTINGS are a tournament's on/off settings, in the order admin shows them. */
export const SETTINGS: { key: Setting; label: string; short: string; hint: string }[] = [
  { key: 'visible', label: 'Shown', short: 'Shown', hint: 'Shown on the site even when not rated' },
  { key: 'rated', label: 'Rated', short: 'Rated', hint: 'In the community ratings, from the next engine run' },
  { key: 'community', label: 'Community leaderboard', short: 'Community', hint: 'Counted in the overall community leaderboard' },
  { key: 'blitz', label: 'Blitz', short: 'Blitz', hint: 'Run from admin as a blitz tournament: its editions, entrants, weekly pairings and results are on the Blitz tab' },
];

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
  return t !== null && isSlug(t) ? t : null;
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

/** divisionsChanged says whether the top divisions box differs from those saved, spaces and blanks aside. */
export function divisionsChanged(text: string, saved: readonly string[]): boolean {
  return divisionText(divisionList(text)) !== divisionText(saved);
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

/** patchReady says whether a patch can be saved: valid (not null) and changing something. */
export function patchReady(patch: object | null): boolean {
  return patch !== null && Object.keys(patch).length > 0;
}

/** penaltySavable says whether a time-out penalty can be saved: readable, and not the one saved. */
export function penaltySavable(value: number | null, saved: number): boolean {
  return value !== null && value !== saved;
}

/** fieldId builds an element id from its parts, joined by dashes, with anything but letters, digits and dashes as _. */
export function fieldId(...parts: string[]): string {
  return parts.join('-').replace(/[^A-Za-z0-9-]/g, '_');
}

/** toggled returns list with item removed if it is there, else added at the end. */
export function toggled(list: readonly string[], item: string): string[] {
  return list.includes(item) ? list.filter((x) => x !== item) : [...list, item];
}

/** noteAction is what a stage's note button does: close the open note, else add one or edit the one there. */
export function noteAction(open: boolean, note: string): 'close' | 'add' | 'edit' {
  if (open) return 'close';
  return note === '' ? 'add' : 'edit';
}

/** tournamentLoad is the page's state from the answers for its tournament and the list of all of them. */
export function tournamentLoad(
  slug: string,
  one: AdminResult<AdminTournament>,
  rows: AdminResult<AdminTournamentRow[]>,
): Load<AdminTournament> {
  if (!one.ok) return { kind: 'error', message: one.status === 404 ? `No tournament is called ${slug}.` : one.message };
  if (!rows.ok) return { kind: 'error', message: rows.message };
  return { kind: 'ready', data: one.data };
}
