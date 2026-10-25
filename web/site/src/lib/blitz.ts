/** The Blitz section's decisions: which edition the address names, its address, how a rating reads, which rounds are the pairings and which the bracket, an edition's tabs, its games as table rows, who sat a round out, and when results last changed. */
import type { components } from '@ttac/shared/api';
import { PAIRING_METHODS, pointsText, resultTag, roundTitle, seededName, turnMark, type BlitzFormat, type PairingMethod, type PairingMethods } from '@ttac/shared/blitz';
import { formatInstant } from '@ttac/shared/time';
import { outcomeNote } from './catalogue-view';
import { pageOf, type GameRow, type GameSeat, type GamesColumn } from './games-table';
import { sortRows, type Sort, type SortColumns } from '@ttac/shared/sort';

type BlitzRound = components['schemas']['BlitzRound'];
type BlitzMatch = components['schemas']['BlitzPublicMatch'];

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

/** An edition page's tabs. */
export type EditionView = 'standings' | 'pairings' | 'players';

/** editionViews lists an edition page's tabs in order: the standings and the pairings once a Swiss round is published, and always the players. */
export function editionViews(rounds: readonly { kind: 'swiss' | 'knockout' }[]): EditionView[] {
  return rounds.some((r) => r.kind === 'swiss') ? ['standings', 'pairings', 'players'] : ['players'];
}

/** BlitzGamesFilter narrows an edition's games: one round (null for every round), one player ('' for all), only those with a replay code, and the page. */
export type BlitzGamesFilter = { round: number | null; player: string; coded: boolean; page: number };

/** YourRow names the signed-in player's game, by its round and the player on its side a, and what its row says to them. */
export type YourRow = { round: number; sideA: string; label: string };

type KeptMatch = { r: BlitzRound; m: BlitzMatch; i: number };

/** BLITZ_GAME_COLUMNS are a blitz games table's sortable columns; players sorts by round, then date, then the highest score. */
export const BLITZ_GAME_COLUMNS: SortColumns<KeptMatch, GamesColumn> = {
  date: { value: ({ m }) => m.game?.finished_at ?? null, first: 'desc' },
  context: { value: ({ r, i }) => [r.number, i], first: 'asc' },
  game: { value: ({ r, i }) => [i, r.number], first: 'asc' },
  players: { value: ({ r, m }) => [r.number, m.game?.finished_at ?? null, m.game === null ? null : -Math.max(m.game.a_culture, m.game.b_culture)], first: 'asc' },
  code: { value: ({ m }) => (coded(m) ? (m.game?.share_code ?? null) : null), first: 'asc' },
};

/** blitzGames lists the matches of rounds, in the order given or by sort, as games-table rows narrowed by filter, with dates on the clocks of zone, the signed-in player's game marked: one page of them, the page and pages, and how many games (byes aside) and how many with a replay code the filter keeps. */
export function blitzGames(rounds: readonly BlitzRound[], format: BlitzFormat, filter: BlitzGamesFilter, zone: string, you: YourRow | null = null, sort: Sort<GamesColumn> = null) {
  const day = new Intl.DateTimeFormat('en-CA', { timeZone: zone, year: 'numeric', month: '2-digit', day: '2-digit' });
  const kept = rounds
    .filter((r) => filter.round === null || r.number === filter.round)
    .flatMap((r) => r.matches.map((m, i) => ({ r, m, i })))
    .filter(({ m }) => (filter.player === '' || m.a === filter.player || m.b === filter.player) && (!filter.coded || coded(m)));
  const rows = sortRows(kept, BLITZ_GAME_COLUMNS, sort).map(({ r, m, i }) => {
    const row = blitzRow(r.number, i + 1, m, format, day);
    return you !== null && r.number === you.round && m.a === you.sideA ? { ...row, you: you.label } : row;
  });
  return { ...pageOf(rows, filter.page), games: kept.filter(({ m }) => m.b !== null).length, coded: kept.filter(({ m }) => coded(m)).length };
}

/** reportTag says who reported a match's game, and whether the opponent confirmed it; '' for an organiser's entry. */
function reportTag(m: BlitzMatch): string {
  if (m.reported_by === null) return '';
  return m.confirmed ? `Reported by ${m.reported_by}, confirmed` : `Reported by ${m.reported_by}`;
}

/** coded says whether a match's game was recorded with a replay code. */
function coded(m: BlitzMatch): boolean {
  return m.game !== null && m.game.share_code !== '';
}

/** blitzRow is game number n of a round as a games-table row: rated players linked with their ratings, seeds in the knockout, each player's turn in a played game, and who reported the game. */
function blitzRow(round: number, n: number, m: BlitzMatch, format: BlitzFormat, day: Intl.DateTimeFormat): GameRow {
  const g = m.game;
  const seat = (name: string, rating: number | null, seed: number | null, score: number | null, note: string, turn: string): GameSeat => ({
    label: seededName(name, seed),
    link: rating === null ? null : name,
    detail: [rating === null ? '' : `(${ratingText(rating)})`, turn].filter((d) => d !== '').join(' '),
    score,
    note,
  });
  const seats = [seat(m.a, m.a_rating, m.a_seed, g?.a_culture ?? null, g === null ? '' : outcomeNote(g.a_outcome), g === null ? '' : turnMark(g.a_turn))];
  if (m.b !== null) seats.push(seat(m.b, m.b_rating, m.b_seed, g?.b_culture ?? null, g === null ? '' : outcomeNote(g.b_outcome), g === null ? '' : turnMark(g.b_turn)));
  return {
    key: `${round}-${n}`,
    date: g === null ? '' : day.format(new Date(g.finished_at)),
    context: roundTitle(round, format),
    where: `Game ${n}`,
    tag: [resultTag(m), reportTag(m)].filter((t) => t !== '').join(' · '),
    unplayed: m.b === null || (m.result !== null && g === null),
    seats,
    code: g === null || g.share_code === '' ? null : g.share_code,
  };
}

/** notPlaying lists who sat out rounds, narrowed to one round (null for every round), in the order given: each round's title and names; a round nobody sat out is left out. */
export function notPlaying(rounds: readonly BlitzRound[], round: number | null, format: BlitzFormat): { round: string; names: string[] }[] {
  return rounds.filter((r) => (round === null || r.number === round) && r.skipped.length > 0).map((r) => ({ round: roundTitle(r.number, format), names: r.skipped }));
}

/** roundOptions names each of rounds for the Round filter, in the order given. */
export function roundOptions(rounds: readonly { number: number }[], format: BlitzFormat): { value: number; label: string }[] {
  return rounds.map((r) => ({ value: r.number, label: roundTitle(r.number, format) }));
}

/** resultsUpdated says when the edition's latest result was recorded, on the clocks of zone; '' before the first. */
export function resultsUpdated(at: string | null, zone: string): string {
  return at === null ? '' : `Results last updated ${formatInstant(at, zone)}.`;
}

/** PairingNotes is what the "How pairings work" dialog says: the edition's methods, the rules every method keeps, and the method each Swiss round so far was paired with. */
export type PairingNotes = { methods: { when: string; label: string; about: string }[]; rules: string[]; paired: { round: string; label: string }[] };

/** pairingNotes explains how an edition's rounds are paired: one method for every Swiss round or round 1's and the later rounds', the rules that hold whatever the method, and the method each Swiss round shown was paired with. */
export function pairingNotes(ed: {
  pairing: PairingMethods;
  points: { bye: number };
  format: BlitzFormat;
  rounds: readonly { number: number; kind: 'swiss' | 'knockout'; method: PairingMethod | null }[];
}): PairingNotes {
  const method = (when: string, m: PairingMethod) => ({ when, label: PAIRING_METHODS[m].label, about: PAIRING_METHODS[m].about });
  const methods = ed.pairing.first === ed.pairing.later ? [method('Every Swiss round', ed.pairing.first)] : [method('Round 1', ed.pairing.first), method('Later rounds', ed.pairing.later)];
  const bye = `${pointsText(ed.points.bye)} ${ed.points.bye > 0 && ed.points.bye <= 1 ? 'point' : 'points'}`;
  const rules = [
    'Nobody meets the same opponent twice.',
    `With an odd number of players, one player gets a bye, worth ${bye}: from the lowest score, someone who has not had one.`,
  ];
  const paired = ed.rounds.flatMap((r) => (r.kind === 'swiss' && r.method !== null ? [{ round: roundTitle(r.number, ed.format), label: PAIRING_METHODS[r.method].label }] : []));
  return { methods, rules, paired };
}
