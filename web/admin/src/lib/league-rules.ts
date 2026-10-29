/** A league's rules in admin: its divisions with their title tiers, and the fewest games a season record needs. */
import type { LeagueDivision } from './admin-api';

/**
 * tiersList reads a league's divisions, highest first, from "name tier" pairs separated by commas ("Emperor 1,
 * King 2"): [] for an empty box, or null where the API would refuse them (no tier, a tier outside 1 to 7, or a
 * division listed twice).
 */
export function tiersList(text: string): LeagueDivision[] | null {
  const out: LeagueDivision[] = [];
  for (const part of text.split(',').map((p) => p.trim()).filter((p) => p !== '')) {
    const m = /^(.*\S)\s+([1-7])$/.exec(part);
    if (m === null || m[1] === undefined || out.some((d) => d.name === m[1])) return null;
    out.push({ name: m[1], tier: Number(m[2]) });
  }
  return out;
}

/** tiersText writes a league's divisions for the box tiersList reads. */
export function tiersText(divisions: readonly LeagueDivision[]): string {
  return divisions.map((d) => `${d.name} ${d.tier}`).join(', ');
}

/** tiersChanged says whether the box holds divisions the API takes that differ from those saved. */
export function tiersChanged(text: string, saved: readonly LeagueDivision[]): boolean {
  const list = tiersList(text);
  return list !== null && tiersText(list) !== tiersText(saved);
}

/** recordGames reads the fewest games a season record needs, a whole number from 0 to 100, or null. */
export function recordGames(text: string): number | null {
  const t = text.trim();
  return /^\d{1,3}$/.test(t) && Number(t) <= 100 ? Number(t) : null;
}
