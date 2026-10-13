/** The Blitz section's decisions: which edition the address names, its address, how a rating reads, and which rounds are the pairings and which the bracket. */

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

/** splitRounds parts an edition's rounds: the Swiss rounds newest first, for the pairings; the knockout rounds in bracket order, first round to final. */
export function splitRounds<R extends { number: number; kind: 'swiss' | 'knockout' }>(rounds: readonly R[]): { swiss: R[]; knockout: R[] } {
  return {
    swiss: rounds.filter((r) => r.kind === 'swiss').sort((a, b) => b.number - a.number),
    knockout: rounds.filter((r) => r.kind === 'knockout').sort((a, b) => a.number - b.number),
  };
}
