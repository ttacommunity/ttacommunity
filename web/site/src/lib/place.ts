/** A finishing place's tone: the medal's for the first three, none after. */

export type PlaceTone = 'gold' | 'silver' | 'bronze' | 'other';

/** placeTone returns the tone a place is drawn in: gold, silver and bronze for 1 to 3, other for any place after. */
export function placeTone(place: number): PlaceTone {
  return place === 1 ? 'gold' : place === 2 ? 'silver' : place === 3 ? 'bronze' : 'other';
}
