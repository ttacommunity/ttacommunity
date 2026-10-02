/** The Tournaments page: which tournament the address asks for, its links, and how its figures are written. */

// The API's pattern for a tournament in a path.
const SLUG = /^[a-z][a-z0-9_]{0,63}$/;

/** tournamentParam returns the tournament the address names (?t=), or null for none or one the API would refuse. */
export function tournamentParam(search: URLSearchParams): string | null {
  const t = search.get('t');
  return t !== null && SLUG.test(t) ? t : null;
}

/** catalogueHref returns the address of the Tournaments list, or of one tournament. */
export function catalogueHref(slug?: string): string {
  return slug === undefined ? '/ratings/tournaments/' : `/ratings/tournaments/?t=${encodeURIComponent(slug)}`;
}

/** spanYears writes the years between two YYYY-MM-DD days: "2024", or "2019 – 2026". */
export function spanYears(first: string, last: string): string {
  const [a, b] = [first.slice(0, 4), last.slice(0, 4)];
  return a === b ? a : `${a} – ${b}`;
}

/** stageName names a stage; "" is a season's only or main stage. */
export function stageName(label: string): string {
  return label === '' ? 'Main stage' : label;
}

/** placeLabel names a podium place as the Hall of Fame's cup honours do. */
export function placeLabel(place: number): string {
  return ['🥇 Champion', '🥈 Runner-Up', '🥉 3rd Place'][place - 1] ?? `${place}th`;
}
