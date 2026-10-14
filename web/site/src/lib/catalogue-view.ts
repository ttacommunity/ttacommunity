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

type Dated = { first_game: string | null; last_game: string | null };

/** seasonDates shows a season's first and last game, or that it has none (a season added for its note). */
export function seasonDates(s: Dated): string {
  return s.first_game === null || s.last_game === null ? 'No games recorded' : `${s.first_game} – ${s.last_game}`;
}

/** activeYears gives the years from the first to the last game of the seasons that have games, or a dash. */
export function activeYears(seasons: readonly Dated[]): string {
  const firsts = seasons.flatMap((s) => (s.first_game === null ? [] : [s.first_game])).sort();
  const lasts = seasons.flatMap((s) => (s.last_game === null ? [] : [s.last_game])).sort();
  const first = firsts[0];
  const last = lasts[lasts.length - 1];
  return first === undefined || last === undefined ? '—' : spanYears(first, last);
}

/** stageName names a stage; "" is a season's only or main stage. */
export function stageName(label: string): string {
  return label === '' ? 'Main stage' : label;
}

/** seasonName names a season in the tournament's word: "Season 24", "Edition 1 (2024)", or a named one such as "Grand Prix 2023" as it is. */
export function seasonName(label: string, word = 'Season'): string {
  return /^\d/.test(label) ? `${word} ${label}` : label;
}

/** plural names more than one season or stage, in the tournament's word: "Seasons", "Editions". */
export function plural(word: string): string {
  return `${word}s`;
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

/** A recent podium as the home page shows it: its tournament, season and the first name at each place. */
export interface PodiumCard {
  slug: string;
  name: string;
  seasonLabel: string;
  seasonWord: string;
  finished: string;
  champion: string | null;
  runnerUp: string | null;
  third: string | null;
}

/** podiumCard gives a recent podium's card for the home page. */
export function podiumCard(p: {
  slug: string;
  name: string;
  season: string;
  season_name: string;
  finished: string;
  podium: { place: number; name: string; final: string }[];
}): PodiumCard {
  const first = (place: number) => placeNames(p.podium, place)[0]?.name ?? null;
  return {
    slug: p.slug,
    name: p.name,
    seasonLabel: p.season,
    seasonWord: p.season_name,
    finished: p.finished,
    champion: first(1),
    runnerUp: first(2),
    third: first(3),
  };
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

/** gameStateLabel marks a game left out of the ratings: never played, an unconfirmed result, or a played game its
 * tournament, season or player count leaves out (not rated); "" for a rated game. */
export function gameStateLabel(state: string, rated: boolean): string {
  if (state === 'never_played') return 'Never played';
  if (state === 'unconfirmed') return 'Unconfirmed result, not rated';
  return rated ? '' : 'Not rated';
}

/** hasPodiums says whether any season has a podium, so the champions table shows its 1st–3rd columns. */
export function hasPodiums(seasons: readonly { podium: readonly unknown[] }[]): boolean {
  return seasons.some((s) => s.podium.length > 0);
}

/** A tournament's Format & Regulations box: one bullet a line, and the CGE edition the text is from, when it is CGE's. */
export type RegulationsBox = { lines: string[]; source: { name: string; url: string } | null };

/** regulationLines splits a text into its non-empty lines, trimmed: the box's bullets. */
export function regulationLines(text: string): string[] {
  return text
    .split('\n')
    .map((l) => l.trim())
    .filter((l) => l !== '');
}

/** regulationsBox gives the text admin wrote, else the latest CGE edition's description (naming it), as bullets; null for neither. */
export function regulationsBox(t: {
  rules: readonly { name: string; description: string; url: string }[];
  regulations?: string | null;
}): RegulationsBox | null {
  const mine = regulationLines(t.regulations ?? '');
  if (mine.length > 0) return { lines: mine, source: null };
  const latest = t.rules[0];
  const lines = regulationLines(latest?.description ?? '');
  return lines.length === 0 ? null : { lines, source: { name: latest.name, url: latest.url } };
}
