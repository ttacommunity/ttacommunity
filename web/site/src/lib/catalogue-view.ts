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

/** seasonName names a season: "Season 24" or "Season 2022 (Age A)", or a named one such as "Grand Prix 2023" as it is. */
export function seasonName(label: string): string {
  return /^\d/.test(label) ? `Season ${label}` : label;
}

/** placeLabel names a podium place as the Hall of Fame's cup honours do. */
export function placeLabel(place: number): string {
  return ['🥇 Champion', '🥈 Runner-Up', '🥉 3rd Place'][place - 1] ?? `${place}th`;
}

/** cgeYears writes the years a CGE tournament ran: "2024 – 2025", "2023", "2026 –" while running, or "". */
export function cgeYears(started: string | null, finished: string | null): string {
  if (started !== null && finished !== null) return spanYears(started, finished);
  if (finished !== null) return finished.slice(0, 4);
  return started === null ? '' : `${started.slice(0, 4)} –`;
}

/** shownName writes a tournament's name, marked "(Not Rated)" when it is shown without being rated. */
export function shownName(name: string, rated: boolean): string {
  return rated ? name : `${name} (Not Rated)`;
}

/** gameWhere writes where a game sits in its season: "Stage 2 · Lower 17 · game 5", leaving out what the game's name lacks. */
export function gameWhere(g: { stage: string; group: string; game: number | null }): string {
  return [g.stage, g.group, g.game === null ? '' : `game ${g.game}`].filter((part) => part !== '').join(' · ');
}

/** outcomeNote marks a seat that did not finish: "resigned" or "timed out"; "" otherwise. */
export function outcomeNote(outcome: string | null): string {
  return outcome === 'resigned' ? 'resigned' : outcome === 'timed_out' ? 'timed out' : '';
}

/** placeNames gives who took a podium place, each with its final named only when the season has several finals. */
export function placeNames(podium: { place: number; name: string; final: string }[], place: number): { name: string; final: string }[] {
  const several = new Set(podium.map((p) => p.final)).size > 1;
  return podium.filter((p) => p.place === place).map((p) => ({ name: p.name, final: several ? p.final : '' }));
}

/** winsLabel counts a ladder winner's top-tier wins ("3.5 wins"), or "" for a single win. */
export function winsLabel(wins: number): string {
  return wins > 1 ? `${wins.toLocaleString('en-US')} wins` : '';
}

/** entryRows gives a tournament card's figures: its years, seasons, games and players. */
export function entryRows(t: { first_game: string; last_game: string; seasons: number; games: number; players: number }): [string, string][] {
  const n = (x: number) => x.toLocaleString('en-US');
  return [
    ['Active History', spanYears(t.first_game, t.last_game)],
    ['Seasons', n(t.seasons)],
    ['Recorded Database Matches', n(t.games)],
    ['Players', n(t.players)],
  ];
}

/** gameStateLabel marks a game left out of the ratings: never played, or an unconfirmed result; "" for a played game. */
export function gameStateLabel(state: string): string {
  return state === 'never_played' ? 'Never played' : state === 'unconfirmed' ? 'Unconfirmed result, not rated' : '';
}
