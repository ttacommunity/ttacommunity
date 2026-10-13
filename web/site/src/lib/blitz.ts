/** The Blitz section's decisions: which edition the address names, its address, and how a rating reads. */

// The API's pattern for a tournament in a path.
const SLUG = /^[a-z][a-z0-9_]{0,63}$/;

/** editionParams returns the tournament slug (?t=) and edition label (?e=) the address names, or null for the list. */
export function editionParams(search: URLSearchParams): { slug: string; edition: string } | null {
  const slug = search.get('t') ?? '';
  const edition = search.get('e') ?? '';
  return SLUG.test(slug) && edition !== '' ? { slug, edition } : null;
}

/** editionHref is the address of an edition's page. */
export function editionHref(slug: string, edition: string): string {
  return `/blitz/?t=${encodeURIComponent(slug)}&e=${encodeURIComponent(edition)}`;
}

/** ratingText shows a rating to the point, or a dash for a player without one. */
export function ratingText(rating: number | null): string {
  return rating === null ? '–' : String(Math.round(rating));
}

/** roundLabel names the round an edition has reached, or says the pairing has not started. */
export function roundLabel(round: number | null): string {
  return round === null ? 'Pairing not started' : `Round ${round}`;
}

/** roundsNewestFirst orders rounds by number, the latest first. */
export function roundsNewestFirst<R extends { number: number }>(rounds: readonly R[]): R[] {
  return [...rounds].sort((a, b) => b.number - a.number);
}
