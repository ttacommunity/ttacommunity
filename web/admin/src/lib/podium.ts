/** A season's podium as admin edits it: 1st to 3rd, each place one or more name boxes (players sharing it). */
import type { components } from '@ttac/shared/api';

export type PodiumEntry = components['schemas']['PodiumEntry'];
export type PodiumSetting = components['schemas']['PodiumSetting'];

/** Places is each place's name boxes, 1st to 3rd. */
export type Places = string[][];

/** podiumRequest builds a season's podium from the boxes, each name with its place, or null when all are empty. */
export function podiumRequest(places: Places, note: string): PodiumSetting | null {
  const podium = places.flatMap((names, i) => names.flatMap((n) => (n.trim() === '' ? [] : [{ place: i + 1, name: n.trim() }])));
  return podium.length === 0 ? null : { podium, note: note.trim() };
}

/** podiumNames lays a podium out as the boxes: each player in their place's, one empty box for a place nobody holds. */
export function podiumNames(podium: PodiumEntry[]): Places {
  const places: Places = [[], [], []];
  for (const e of podium) places[e.place - 1]?.push(e.name);
  return places.map((names) => (names.length === 0 ? [''] : names));
}

/** sharePlace adds an empty box to the place at index i, for another player sharing it. */
export function sharePlace(places: Places, i: number): Places {
  return places.map((names, j) => (j === i ? [...names, ''] : names));
}

/** unsharePlace removes box k of the place at index i; a place's first box (k 0) stays. */
export function unsharePlace(places: Places, i: number, k: number): Places {
  return places.map((names, j) => (j === i && k > 0 ? names.filter((_, m) => m !== k) : names));
}

/** Origin is where the boxes' names come from: set by hand, recorded, a final stage's games, or nowhere. */
export type Origin = 'admin' | 'recorded' | 'final_stage' | 'none';

/** podiumStart returns the boxes' first contents: the podium set by hand, else the suggestion, and its origin. */
export function podiumStart(saved: PodiumEntry[], suggested: PodiumEntry[], from: string): { names: Places; origin: Origin } {
  if (saved.length > 0) return { names: podiumNames(saved), origin: 'admin' };
  if (suggested.length === 0 || (from !== 'recorded' && from !== 'final_stage')) return { names: podiumNames([]), origin: 'none' };
  return { names: podiumNames(suggested), origin: from };
}

// What the note says of a podium nobody has set by hand.
const ORIGIN_NOTE: Record<Origin, string> = {
  admin: '',
  recorded: 'As recorded by the organisers',
  final_stage: "From the final stage's games",
  none: '',
};

/** podiumNoteStart is the note box's first contents: the saved note, else where the suggested names came from. */
export function podiumNoteStart(saved: string, origin: Origin): string {
  return saved || ORIGIN_NOTE[origin];
}
