/** A division's level in its tournament's ladder, so a division tag takes its level's colour: level 1 the top, the same colour at the same level in every ladder. */

// The title tiers, top first: the International and Intermezzo divisions, and every tournament without a ladder of its own.
const TIERS = ['grandmaster', 'master', 'diamond', 'platinum', 'gold', 'silver', 'bronze', 'wood'];

// Tournaments with a ladder of their own, by slug.
const LADDERS: Record<string, string[]> = {
  royal_league: ['emperor', 'king', 'prince', 'duke', 'marquess', 'count', 'viscount', 'baron'],
};

/** divisionLevel returns name's level from 1 (the top) in the ladder of tournament (a slug or a name), or null for a division not in it. */
export function divisionLevel(name: string, tournament: string): number | null {
  const ladder = LADDERS[tournament.trim().toLowerCase().replace(/\s+/g, '_')] ?? TIERS;
  const i = ladder.indexOf(name.trim().toLowerCase().replace(/\s+\d+$/, ''));
  return i < 0 ? null : i + 1;
}

/** divisionTagClass returns the classes of a tag for the division name in tournament: its level's colour, or the plain tag. */
export function divisionTagClass(name: string, tournament: string): string {
  const level = divisionLevel(name, tournament);
  return level === null ? 'tag' : `tag tag-level-${level}`;
}
