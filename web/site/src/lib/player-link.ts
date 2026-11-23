/** A player's name's looks: the one name look, a finishing place's colour, or set back. */

/** Where a player finished: first, second and third in the medals' colours; last set back. */
export type Place = 1 | 2 | 3 | 'last';

/** playerLinkClass returns a name's classes: the name look, then its place's colour, or quiet for a name that is not the subject (the other seats in a game between two players). */
export function playerLinkClass({ place, quiet }: { place?: Place; quiet?: boolean }): string {
  if (place !== undefined) return `player-link player-place-${place}`;
  return quiet ? 'player-link player-quiet' : 'player-link';
}
