/** The circuits: groups of several tournaments with a Hall of Fame page of their own, as the site and admin know them. */

/** CIRCUITS are the circuits' slugs, in the series bar's order. */
export const CIRCUITS = ['grand_slams', 'ladders'] as const;

/** A circuit's slug. */
export type Circuit = (typeof CIRCUITS)[number];

/** isCircuit says whether a tournament or group slug is a circuit's, so it has a Hall of Fame page. */
export function isCircuit(slug: string): slug is Circuit {
  return (CIRCUITS as readonly string[]).includes(slug);
}
