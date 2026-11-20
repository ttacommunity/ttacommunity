/** The Blitz section's decisions: how a rating reads, which rounds are the pairings and which the bracket, an edition's tabs, its games as table rows, who sat a round out, and when results last changed. */
import type { components } from '@ttac/shared/api';
import { bestOfScore, pointsText, resultTag, roundTitle, seededName, turnMark, type BlitzFormat, type PairingMethod, type PairingMethods, methodText, orderLabel, ranksFixedBy, roundLine, type PairingOrder } from '@ttac/shared/blitz';
import { formatInstant } from '@ttac/shared/time';
import { outcomeNote } from './catalogue-view';
import { pageOf, type GameRow, type GameSeat, type GamesColumn } from './games-table';
import { sortRows, type Sort, type SortColumns } from '@ttac/shared/sort';

type BlitzRound = components['schemas']['BlitzRound'];
type BlitzMatch = components['schemas']['BlitzPublicMatch'];
type BlitzGame = BlitzMatch['games'][number];
type BlitzMatchReport = components['schemas']['BlitzMatchReport'];
type MyBlitz = components['schemas']['MyBlitz'];

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

/** KeptMatch is one row's game: its round, its match and the match's place in the round, and the game (null for none played yet). */
type KeptMatch = { r: BlitzRound; m: BlitzMatch; i: number; g: BlitzGame | null };

/** BLITZ_GAME_COLUMNS are a blitz games table's sortable columns; players sorts by round, then date, then the highest score. */
export const BLITZ_GAME_COLUMNS: SortColumns<KeptMatch, GamesColumn> = {
  date: { value: ({ g }) => g?.game.finished_at ?? null, first: 'desc' },
  context: { value: ({ r, i, g }) => [r.number, i, g?.number ?? 0], first: 'asc' },
  game: { value: ({ r, i, g }) => [i, r.number, g?.number ?? 0], first: 'asc' },
  players: { value: ({ r, g }) => [r.number, g?.game.finished_at ?? null, g === null ? null : -Math.max(g.game.a_culture, g.game.b_culture)], first: 'asc' },
  code: { value: ({ g }) => (coded(g) ? (g?.game.share_code ?? null) : null), first: 'asc' },
};

/** gamesOf lists a match's rows: each game of a best of 3, or the one game (null while unplayed) of any other match. */
function gamesOf(m: BlitzMatch): (BlitzGame | null)[] {
  return m.best_of === 3 && m.games.length > 0 ? m.games : [m.games[0] ?? null];
}

/** blitzGames lists the games of rounds' matches (one row for a match not yet played), in the order given or by sort, as games-table rows narrowed by filter, with dates on the clocks of zone, the signed-in player's game marked and who reported each game, from reports: one page of them, the page and pages, and how many games (byes aside) and how many with a replay code the filter keeps. */
export function blitzGames(rounds: readonly BlitzRound[], format: BlitzFormat, filter: BlitzGamesFilter, zone: string, you: YourRow | null = null, sort: Sort<GamesColumn> = null, reports: readonly BlitzMatchReport[] = []) {
  const day = new Intl.DateTimeFormat('en-CA', { timeZone: zone, year: 'numeric', month: '2-digit', day: '2-digit' });
  const kept = rounds
    .filter((r) => filter.round === null || r.number === filter.round)
    .flatMap((r) => r.matches.flatMap((m, i) => gamesOf(m).map((g) => ({ r, m, i, g }))))
    .filter(({ m, g }) => (filter.player === '' || m.a === filter.player || m.b === filter.player) && (!filter.coded || coded(g)));
  const rows = sortRows(kept, BLITZ_GAME_COLUMNS, sort).map(({ r, m, i, g }) => {
    const report = reports.find((p) => p.round === r.number && p.a === m.a && p.game === (g?.number ?? 1)) ?? null;
    const row = blitzRow(r.number, i + 1, m, g, format, day, report);
    return you !== null && r.number === you.round && m.a === you.sideA ? { ...row, you: you.label } : row;
  });
  return { ...pageOf(rows, filter.page), games: kept.filter(({ m }) => m.b !== null).length, coded: kept.filter(({ g }) => coded(g)).length };
}

/** reportTag says who reported a match's game, and whether the opponent confirmed it; '' for an organiser's entry or a viewer not told. */
function reportTag(report: BlitzMatchReport | null): string {
  if (report === null) return '';
  return report.confirmed ? `Reported by ${report.reported_by}, confirmed` : `Reported by ${report.reported_by}`;
}

/** asksToSignIn says whether the pairings ask a visitor to sign in to report: once the page knows nobody is signed in (seen null; undefined while it asks), while the edition has no champion. */
export function asksToSignIn(seen: MyBlitz | null | undefined, champion: string | null): boolean {
  return seen === null && champion === null;
}

/** coded says whether a game was recorded with a replay code. */
function coded(g: BlitzGame | null): boolean {
  return g !== null && g.game.share_code !== '';
}

/** bestOfTag tags a game of a best of 3 with its number and, on its last game once the match is decided, the score. */
function bestOfTag(m: BlitzMatch, game: BlitzGame): string {
  const last = m.result !== null && game.number === m.games.length;
  return `Best of 3, game ${game.number}` + (last ? `: ${bestOfScore(m)}` : '');
}

/** blitzRow is match number n of a round, or one of its games, as a games-table row: rated players linked with their ratings, seeds in the knockout, each player's turn in a played game, and who reported the game (report null for nobody, or a viewer not told). */
function blitzRow(round: number, n: number, m: BlitzMatch, game: BlitzGame | null, format: BlitzFormat, day: Intl.DateTimeFormat, report: BlitzMatchReport | null): GameRow {
  const g = game?.game ?? null;
  const bestOf = m.best_of === 3 && game !== null;
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
    key: bestOf ? `${round}-${n}-${game.number}` : `${round}-${n}`,
    date: g === null ? '' : day.format(new Date(g.finished_at)),
    context: roundTitle(round, format),
    where: `Game ${n}`,
    tag: [bestOf ? bestOfTag(m, game) : resultTag({ ...m, game: g }), reportTag(report)].filter((t) => t !== '').join(' · '),
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
export type PairingNotes = { methods: { when: string; label: string; about: string }[]; rules: string[] };

/** pairingNotes explains how an edition's rounds are paired: round 1's method and order and, with more than one Swiss round, the later rounds', each in its own words; and the rules that hold whatever the method, how the starting ranks are fixed among them. */
export function pairingNotes(ed: {
  pairing: PairingMethods;
  ranks_fixed_by: 'rating' | 'lot' | null;
  points: { bye: number };
  format: BlitzFormat;
}): PairingNotes {
  const method = (when: string, m: PairingMethod, round: 'first' | 'later', o: PairingOrder) => ({ when, label: roundLine(m, o), about: methodText(m, round, o) });
  const methods = [method('Round 1', ed.pairing.first, 'first', ed.pairing.first_order)];
  if (ed.format.swiss_rounds > 1) methods.push(method('Later rounds', ed.pairing.later, 'later', ed.pairing.later_order));
  const bye = `${pointsText(ed.points.bye)} ${ed.points.bye > 0 && ed.points.bye <= 1 ? 'point' : 'points'}`;
  const rules = [
    'Nobody meets the same opponent twice.',
    `With an odd number of players, one player gets a bye, worth ${bye}: from the lowest score, someone who has not had one.`,
  ];
  if (ed.ranks_fixed_by !== null) rules.push(`Starting ranks were fixed for everyone when round 1 was paired, ${orderLabel(ed.ranks_fixed_by)}: the Players tab shows them.`);
  else rules.push(`Starting ranks are fixed for everyone when round 1 is paired, ${orderLabel(ranksFixedBy(ed.pairing))}; a late entrant is slotted in the same way.`);
  return { methods, rules };
}
