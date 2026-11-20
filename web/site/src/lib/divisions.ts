/** A division's level, so a division tag takes its level's colour: the same level, the same colour, in each ladder. */

// Top to bottom: the International and Intermezzo ladder, and Royal League's (whose divisions carry a number, "King 2").
const LADDERS = [
  ['grandmaster', 'master', 'diamond', 'platinum', 'gold', 'silver', 'bronze'],
  ['emperor', 'king', 'prince', 'duke', 'marquess', 'count'],
];

/** divisionLevel returns name's level from 1 (the top), or null for a division in no ladder. */
export function divisionLevel(name: string): number | null {
  const rank = name.trim().toLowerCase().replace(/\s+\d+$/, '');
  for (const ladder of LADDERS) {
    const i = ladder.indexOf(rank);
    if (i >= 0) return i + 1;
  }
  return null;
}

/** divisionTagClass returns the classes of a tag for the division name: its level's colour, or the plain tag. */
export function divisionTagClass(name: string): string {
  const level = divisionLevel(name);
  return level === null ? 'tag' : `tag tag-level-${level}`;
}
