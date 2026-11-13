/** A tournament's settings page in admin: its address, and the settings it edits. */
import type { Load } from '@ttac/shared/load';
import { isSlug } from '@ttac/shared/ratings';
import type { AdminResult, AdminTournament, AdminTournamentRow } from './admin-api';

export type Setting = 'visible' | 'rated' | 'superblitz_rated' | 'community' | 'blitz' | 'bracket';

/** SETTINGS are a tournament's on/off settings, in the order admin shows them. */
export const SETTINGS: { key: Setting; label: string; short: string; hint: string }[] = [
  { key: 'visible', label: 'Shown', short: 'Shown', hint: 'Shown on the site even when not rated' },
  { key: 'rated', label: 'Rated (Standard)', short: 'Standard', hint: 'In the Standard ratings, from the next engine run' },
  { key: 'superblitz_rated', label: 'Rated (SuperBlitz)', short: 'SuperBlitz', hint: 'In the SuperBlitz ratings (and Standard + SuperBlitz), from the next engine run' },
  { key: 'community', label: 'Community leaderboard', short: 'Community', hint: 'Counted in the overall community leaderboard' },
  { key: 'blitz', label: 'Blitz', short: 'Blitz', hint: 'Run from admin as a blitz tournament: its editions, entrants, weekly pairings and results are on the Blitz tab' },
  { key: 'bracket', label: 'Shown as a bracket', short: 'Bracket', hint: 'Each season shown on the site as a knockout bracket: its stages as rounds of groups or matches, up to the final' },
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

/** seasonBox is a season's box as it takes effect: the season's own setting while the tournament's is on, else
 * unticked and locked (the season's setting is kept for when the tournament's is turned on). */
export function seasonBox(tournamentOn: boolean, seasonOn: boolean): { checked: boolean; locked: boolean } {
  return tournamentOn ? { checked: seasonOn, locked: false } : { checked: false, locked: true };
}

/** settingBox is a setting's box as it takes effect: Shown as a bracket is unticked and locked, with a note, while the
 * tournament is neither shown nor rated, as the site shows it only then; every other setting is as saved. */
export function settingBox(key: Setting, t: Record<Setting, boolean>): { checked: boolean; locked: boolean; note: string } {
  if (key !== 'bracket') return { checked: t[key], locked: false, note: '' };
  const box = seasonBox(t.visible || t.rated || t.superblitz_rated, t.bracket);
  return { ...box, note: box.locked ? "(the tournament isn't shown or rated: tick Shown or Rated first)" : '' };
}

/** settingPatch builds the settings update for a checkbox, keeping Standard and SuperBlitz mutually exclusive. */
export function settingPatch(key: Setting, checked: boolean): Partial<Record<Setting, boolean>> {
  if (key === 'rated' && checked) return { rated: true, superblitz_rated: false };
  if (key === 'superblitz_rated' && checked) return { superblitz_rated: true, rated: false };
  return { [key]: checked };
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
