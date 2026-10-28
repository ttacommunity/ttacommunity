/** What both sites say about a blitz tournament: an edition's address, a zone's offset, a format, its points, how a match's result and turn order read, how a folded round is summed up, how a round and a seeded player are named, how the standings share places, and how rounds are paired. */
import type { components } from '../api/schema';
import { isSlug } from './ratings';
import type { SortColumns } from './sort';

/** editionParams returns the tournament slug (?t=) and edition label (?e=) the address names, or null for none. */
export function editionParams(search: URLSearchParams): { slug: string; edition: string } | null {
  const slug = search.get('t') ?? '';
  const edition = search.get('e') ?? '';
  return isSlug(slug) && edition !== '' ? { slug, edition } : null;
}

/** editionHref is the address of an edition's page. */
export function editionHref(slug: string, edition: string): string {
  return `/blitz/?t=${encodeURIComponent(slug)}&e=${encodeURIComponent(edition)}`;
}

export type BlitzFormat = components['schemas']['BlitzFormat'];
export type BlitzResult = components['schemas']['BlitzResult'];
export type BlitzPlayedGame = components['schemas']['BlitzPlayedGame'];
export type BlitzPoints = components['schemas']['BlitzPoints'];

/** MatchResultView is what a match's result line needs: both names (b null for the bye), the result, the game played and whether it was ruled. */
export type MatchResultView = { a: string; b: string | null; result: BlitzResult | null; game: BlitzPlayedGame | null; ruled: boolean };

/** endings names a player who resigned or timed out (", Cy resigned"), '' when both finished. */
function endings(m: MatchResultView, game: BlitzPlayedGame): string {
  const ended = (name: string | null, outcome: BlitzPlayedGame['a_outcome']) =>
    outcome === 'finished' ? '' : `, ${name ?? '?'} ${outcome === 'resigned' ? 'resigned' : 'timed out'}`;
  return ended(m.a, game.a_outcome) + ended(m.b, game.b_outcome);
}

/** scoreText writes a game's cultures as "250–200", the winner's first when b won, with how it ended. */
function scoreText(m: MatchResultView, game: BlitzPlayedGame): string {
  const score = m.result === 'b' ? `${game.b_culture}–${game.a_culture}` : `${game.a_culture}–${game.b_culture}`;
  return score + endings(m, game);
}

/** namedScores writes each player's culture by name ("Ann 250, Cy 200"), for a game whose result a ruling replaced. */
function namedScores(m: MatchResultView, game: BlitzPlayedGame): string {
  return `${m.a} ${game.a_culture}, ${m.b ?? '?'} ${game.b_culture}` + endings(m, game);
}

/** drawnGame says whether a game ended level: both players finished on the same culture. */
function drawnGame(g: BlitzPlayedGame): boolean {
  return g.a_outcome === 'finished' && g.b_outcome === 'finished' && g.a_culture === g.b_culture;
}

/** resultSummary says how a match ended: "bye", "" while undecided, else who won (or a draw, or a double loss), the score with the winner's first, "by ruling" when the organisers overruled a game, "by forfeit" when they decided it with no game played, and who went through as the higher seed when a knockout game was drawn. */
export function resultSummary(m: MatchResultView): string {
  if (m.b === null) return 'bye';
  if (m.result === null) return '';
  const who = { a: `${m.a} won`, b: `${m.b} won`, draw: 'Draw', double_loss: 'Double loss' }[m.result];
  if (!m.ruled && m.game !== null && drawnGame(m.game) && (m.result === 'a' || m.result === 'b')) {
    return `Draw ${scoreText(m, m.game)}: ${m.result === 'a' ? m.a : m.b} through as the higher seed`;
  }
  if (!m.ruled) return m.game === null ? who : `${who} ${scoreText(m, m.game)}`;
  if (m.game !== null) return `${who} by ruling (game: ${namedScores(m, m.game)})`;
  return { a: `${m.a} won by forfeit`, b: `${m.b} won by forfeit`, draw: 'Draw by ruling', double_loss: 'Both forfeited' }[m.result];
}

/** resultTag tags a match in a games table, where the scores tell a played game's result: "Bye", "To be played", who went through a drawn knockout game as the higher seed, a ruling over a game, or a forfeit (no game played); '' otherwise. */
export function resultTag(m: MatchResultView): string {
  if (m.b === null) return 'Bye';
  if (m.result === null) return 'To be played';
  const winner = m.result === 'a' ? m.a : m.result === 'b' ? m.b : null;
  if (!m.ruled) return m.game !== null && drawnGame(m.game) && winner !== null ? `${winner} through as the higher seed` : '';
  if (m.game !== null) return { a: `${m.a} won`, b: `${m.b} won`, draw: 'Draw', double_loss: 'Double loss' }[m.result] + ' by ruling';
  return { a: `${m.a} won by forfeit`, b: `${m.b} won by forfeit`, draw: 'Draw by ruling', double_loss: 'Both forfeited' }[m.result];
}

/** turnMark marks a player's turn: P1 for who went first, P? when it was not recorded. */
export function turnMark(turn: number | null): string {
  return turn === null ? 'P?' : `P${turn}`;
}

/** turnOrder says who played in which turn ("Turn order: P1 Cy, P2 Ann"), that it is unknown, or '' with no game. */
export function turnOrder(m: MatchResultView): string {
  const g = m.game;
  if (g === null || m.b === null) return '';
  if (g.a_turn === null || g.b_turn === null) return 'Turn order unknown';
  const players = [
    { name: m.a, turn: g.a_turn },
    { name: m.b, turn: g.b_turn },
  ].sort((x, y) => x.turn - y.turn);
  return 'Turn order: ' + players.map((p) => `${turnMark(p.turn)} ${p.name}`).join(', ');
}

/** offsetLabel shows an offset the API sends (+02:00) as UTC+2, UTC-4 or UTC+5:30; UTC for none, '' for unknown. */
export function offsetLabel(offset: string): string {
  const m = /^([+-])(\d\d):(\d\d)$/.exec(offset);
  if (m === null) return '';
  const [, sign, hours, minutes] = m;
  const h = Number(hours);
  if (h === 0 && minutes === '00') return 'UTC';
  return `UTC${sign}${h}${minutes === '00' ? '' : `:${minutes}`}`;
}

/** zoneLabel shows a player's zone with its offset now, as "UTC+2 · Europe/Berlin", or the zone alone. */
export function zoneLabel(zone: string, offset: string): string {
  const o = offsetLabel(offset);
  return o === '' ? zone : `${o} · ${zone}`;
}

/** roundSummary sums a round up for when it is folded away: its games, how many have a result, and its bye. */
export function roundSummary(matches: readonly { b: unknown; result: unknown }[]): string {
  const games = matches.filter((m) => m.b !== null);
  if (matches.length === 0) return 'no matches';
  const played = games.filter((m) => m.result !== null).length;
  const done = played === games.length ? 'all played' : played === 0 ? 'none played' : `${played} played`;
  const bye = games.length < matches.length ? ', and a bye' : '';
  return `${games.length} ${games.length === 1 ? 'game' : 'games'}, ${done}${bye}`;
}

/** roundShown says whether a round's matches show: the newest round's unless its button hid them, an earlier one's only once its button showed them. toggled holds the round numbers whose button was pressed. */
export function roundShown(round: number, newest: number, toggled: ReadonlySet<number>): boolean {
  return (round === newest) !== toggled.has(round);
}

/** formatLabel describes an edition's format in words. */
export function formatLabel(f: BlitzFormat): string {
  const swiss = `${f.swiss_rounds} Swiss round${f.swiss_rounds === 1 ? '' : 's'}`;
  return f.knockout_size === 0 ? `${swiss}, no knockout` : `${swiss}, then a top-${f.knockout_size} knockout`;
}

export type PairingMethod = components['schemas']['BlitzPairingMethod'];
export type PairingMethods = components['schemas']['BlitzPairingMethods'];

/** FIRST_METHODS are the methods round 1 may be paired by, in the order offered. */
export const FIRST_METHODS: readonly PairingMethod[] = ['dutch', 'timezone', 'dutch-timezone', 'random'];
/** LATER_METHODS are the methods the rounds after round 1 may be paired by, in the order offered. */
export const LATER_METHODS: readonly PairingMethod[] = ['dutch', 'timezone', 'dutch-timezone', 'random'];

export type PairingOrder = PairingMethods['first_order'];

/** ORDERS are what a method pairs by where it leaves a choice, in the order offered: the Dutch system's starting ranks, the tiebreak among equally close time zones. */
export const ORDERS: readonly { value: PairingOrder; label: string }[] = [
  { value: 'rating', label: 'By rating' },
  { value: 'lot', label: 'By lot' },
  { value: 'random', label: 'Random' },
];

/** orderLabel names an order in a sentence: "by rating", "by lot", "random". */
export function orderLabel(o: PairingOrder): string {
  return ORDERS.find((x) => x.value === o)?.label.toLowerCase() ?? o;
}

/** needsOrder says whether a method pairs by an order at all: every method but Random. */
export function needsOrder(m: PairingMethod): boolean {
  return m !== 'random';
}

/** ranksFixedBy says what fixes the starting ranks when round 1 is paired: the first of the two orders that is rating or lot, else a lot. */
export function ranksFixedBy(p: PairingMethods): 'rating' | 'lot' {
  for (const o of [p.first_order, p.later_order]) if (o === 'rating' || o === 'lot') return o;
  return 'lot';
}

/** PAIRING_METHODS names each pairing method; methodText says how it pairs a round. */
export const PAIRING_METHODS: Record<PairingMethod, { label: string }> = {
  dutch: { label: "FIDE's Dutch system" },
  'dutch-timezone': { label: 'Dutch, time zones as tiebreak' },
  timezone: { label: 'Closest time zones' },
  rating: { label: 'Closest ratings' },
  random: { label: 'Random' },
};

/** roundLine names a method with its order, as a round's one-line label: "Closest time zones, by lot"; Random alone. */
export function roundLine(m: PairingMethod, o: PairingOrder): string {
  return needsOrder(m) ? `${PAIRING_METHODS[m].label}, ${orderLabel(o)}` : PAIRING_METHODS[m].label;
}

/** methodText says in plain words how a method pairs round 1 (first: nobody has points yet) or a later round (later: within each score group) by the order. */
export function methodText(m: PairingMethod, when: 'first' | 'later', o: PairingOrder): string {
  const who = when === 'first' ? 'Everyone meets' : 'Players on the same points meet';
  const ranks = { rating: 'by rating', lot: 'drawn by lot', random: 'drawn afresh for this round' }[o];
  const tie = { rating: 'the closest in rating', lot: 'the closest in the lot', random: 'chance' }[o];
  const group = when === 'first' ? 'The players are' : 'Players on the same points meet. Each score group is';
  switch (m) {
    case 'dutch':
      return `${group} ordered by starting rank, ${ranks}, and split in two, and the top half plays the bottom half: the first against the first of the bottom half, the second against the second, and so on. Pairs change only to avoid a rematch${when === 'first' ? '' : ', and a player left over moves down to the next group'}.`;
    case 'dutch-timezone':
      return `${group} ordered by starting rank, ${ranks}, and the top half plays the bottom half; within that, each player meets the opponent in the bottom half closest to their time zone.`;
    case 'timezone':
      return `${who} the opponent closest to their time zone; among equally close ones, ${tie} decides.`;
    case 'rating':
      return `${who} the opponent closest in rating.`;
    case 'random':
      return 'Opponents are drawn by chance.';
  }
}

/** pairingSummary says in one line how an edition's rounds are paired: one method and order for every round, or round 1's and the later rounds'. */
export function pairingSummary(p: PairingMethods): string {
  const first = roundLine(p.first, p.first_order);
  const later = roundLine(p.later, p.later_order);
  return first === later ? `${first}, every round` : `Round 1: ${first}; later rounds: ${later}`;
}

/** fixedByLabel says what fixed the starting ranks: "by lot" or "by rating"; null before round 1. */
export function fixedByLabel(fixedBy: 'rating' | 'lot' | null): string | null {
  return fixedBy === null ? null : orderLabel(fixedBy);
}

/** startLabel is the starting-rank column's heading: "Lot" when a draw fixed the ranks, "Seed" when ratings did; null before round 1. */
export function startLabel(fixedBy: 'rating' | 'lot' | null): string | null {
  return fixedBy === null ? null : fixedBy === 'lot' ? 'Lot' : 'Seed';
}

/** pointsText writes points in halves as a player reads them: 0, ½, 1, 2½. */
export function pointsText(n: number): string {
  const whole = Math.floor(n);
  if (n === whole) return String(n);
  return whole === 0 ? '½' : `${whole}½`;
}

/** pointsList lists what each outcome is worth: "win 3, draw 1, loss 0, forfeit win 3, forfeit loss 0, bye 3, skip 0". */
export function pointsList(p: BlitzPoints): string {
  const outcomes: [string, number][] = [
    ['win', p.win],
    ['draw', p.draw],
    ['loss', p.loss],
    ['forfeit win', p.forfeit_win],
    ['forfeit loss', p.forfeit_loss],
    ['bye', p.bye],
    ['skip', p.skip],
  ];
  return outcomes.map(([name, n]) => `${name} ${pointsText(n)}`).join(', ');
}

/** scoringSentence says how the standings score: FIDE's in the words the site has always used, an edition's own with every outcome. */
export function scoringSentence(p: BlitzPoints, fide: boolean): string {
  return fide ? 'A win is 1 point, a draw a half, a bye 1.' : `This edition's points: ${pointsList(p)}.`;
}

/** scoringLine is the scoring in a glance: "Win 1 · Draw ½ · Bye 1" for FIDE's, every outcome for an edition's own. */
export function scoringLine(p: BlitzPoints, fide: boolean): string {
  if (fide) return `Win ${pointsText(p.win)} · Draw ${pointsText(p.draw)} · Bye ${pointsText(p.bye)}`;
  return `Win ${pointsText(p.win)} · Draw ${pointsText(p.draw)} · Loss ${pointsText(p.loss)} · Forfeit win ${pointsText(p.forfeit_win)} · Forfeit loss ${pointsText(p.forfeit_loss)} · Bye ${pointsText(p.bye)} · Skip ${pointsText(p.skip)}`;
}

/** knockoutStage names a knockout round by how many still play in it (Final, Semi-finals, Quarter-finals, Round of 16), or null for a round outside the knockout. */
export function knockoutStage(round: number, f: BlitzFormat): string | null {
  const k = round - f.swiss_rounds;
  if (k < 1 || f.knockout_size === 0) return null;
  const players = f.knockout_size / 2 ** (k - 1);
  if (players < 2) return null;
  return { 2: 'Final', 4: 'Semi-finals', 8: 'Quarter-finals' }[players] ?? `Round of ${players}`;
}

/** roundTitle names a round: "Round 3", or "Round 9 · Quarter-finals" in the knockout. */
export function roundTitle(round: number, f: BlitzFormat): string {
  const stage = knockoutStage(round, f);
  return stage === null ? `Round ${round}` : `Round ${round} · ${stage}`;
}

/** seededName writes a knockout player with their seed, as "(1) Ann", or the name alone without one. */
export function seededName(name: string, seed: number | null): string {
  return seed === null ? name : `(${seed}) ${name}`;
}

/** reachedLabel says how far an edition has got: "Pairing not started", the latest published round's title, or "Finished: champion Ann". */
export function reachedLabel(round: number | null, f: BlitzFormat, champion: string | null): string {
  if (champion !== null) return `Finished: champion ${champion}`;
  return round === null ? 'Pairing not started' : roundTitle(round, f);
}

/** lastTieBreak names what orders the players every tie-break leaves level: "the lot" or "the seed" by what fixed the starting ranks, "the draw" before round 1. */
export function lastTieBreak(fixedBy: 'rating' | 'lot' | null): string {
  return fixedBy === 'lot' ? 'the lot' : fixedBy === 'rating' ? 'the seed' : 'the draw';
}

/** drawLabel says which ranks the lot, the seed or the draw ordered (lastTieBreak), for a standing in a shared place: "Ranks 8–9 were ordered by the lot"; null for a rank held alone. */
export function drawLabel(s: { place_from: number; place_to: number }, fixedBy: 'rating' | 'lot' | null): string | null {
  return s.place_from === s.place_to ? null : `Ranks ${s.place_from}–${s.place_to} were ordered by ${lastTieBreak(fixedBy)}`;
}

/** TIE_BREAKS are the steps that separate players on the same points, in the order they apply, each said in plain words. */
export const TIE_BREAKS: readonly { label: string; about: string }[] = [
  { label: 'Direct Encounter', about: 'The games among the tied players, when they have all met each other.' },
  { label: 'Buchholz Cut 1', about: "The sum of the opponents' scores, the lowest left out." },
  { label: 'Buchholz', about: "The sum of all the opponents' scores." },
  { label: 'Sonneborn-Berger', about: 'The scores of the opponents beaten, plus half the scores of those drawn.' },
  { label: "Opponents' Buchholz", about: "The sum of the opponents' Buchholz." },
  { label: "Opponents' Sonneborn-Berger", about: "The sum of the opponents' Sonneborn-Berger." },
  { label: 'The lot', about: 'The starting ranks fixed when round 1 was paired, the lower first: a draw of lots, or the seed when they follow the ratings. Before round 1, a draw fixed for the edition.' },
];

/** UNPLAYED_NOTE says how a bye, a forfeit or a missed round counts in the tie-breaks. */
export const UNPLAYED_NOTE = "A bye, a forfeit or a missed round counts as a game against an opponent on the player's own score.";

export type BlitzStanding = components['schemas']['BlitzStanding'];

/** STANDING_COLUMNS are the standings tables' sortable columns, on the site and in admin. */
export const STANDING_COLUMNS: SortColumns<BlitzStanding, 'rank' | 'player' | 'start' | 'points' | 'played' | 'byes' | 'cut1' | 'buchholz' | 'sb'> = {
  rank: { value: (s) => s.rank, first: 'asc' },
  start: { value: (s) => s.starting_rank, first: 'asc' },
  player: { value: (s) => s.name, first: 'asc' },
  points: { value: (s) => s.points, first: 'desc' },
  played: { value: (s) => s.games, first: 'desc' },
  byes: { value: (s) => s.byes, first: 'desc' },
  cut1: { value: (s) => s.buchholz_cut1, first: 'desc' },
  buchholz: { value: (s) => s.buchholz, first: 'desc' },
  sb: { value: (s) => s.sonneborn_berger, first: 'desc' },
};

/** EditionRow is what an editions list shows of an edition, on the site and in admin. */
export type EditionRow = { name: string; edition: string; format: BlitzFormat; round: number | null; champion: string | null; entrants: number };

/** EDITION_COLUMNS are an editions list's sortable columns; reached puts a champion first, then the latest round. */
export const EDITION_COLUMNS: SortColumns<EditionRow, 'tournament' | 'edition' | 'format' | 'reached' | 'players'> = {
  tournament: { value: (e) => e.name, first: 'asc' },
  edition: { value: (e) => e.edition, first: 'desc' },
  format: { value: (e) => [e.format.swiss_rounds, e.format.knockout_size], first: 'desc' },
  reached: { value: (e) => [e.champion === null ? 0 : 1, e.round], first: 'desc' },
  players: { value: (e) => e.entrants, first: 'desc' },
};

/** offsetMinutes reads a UTC offset ("+05:30") as minutes east of UTC; null for none (''). */
export function offsetMinutes(offset: string): number | null {
  const m = /^([+-])(\d{2}):(\d{2})$/.exec(offset);
  if (m === null) return null;
  return (m[1] === '-' ? -1 : 1) * (Number(m[2]) * 60 + Number(m[3]));
}

/** EntrantRow is what an entrants table shows of every entrant, on the site and in admin. */
export type EntrantRow = { name: string; zone: string; utc_offset: string; rating: number | null; starting_rank: number | null };

/** ENTRANT_COLUMNS are an entrants table's sortable columns; zone sorts west to east, then by the zone's name. */
export const ENTRANT_COLUMNS: SortColumns<EntrantRow, 'player' | 'zone' | 'rating' | 'start'> = {
  player: { value: (e) => e.name, first: 'asc' },
  zone: { value: (e) => [offsetMinutes(e.utc_offset), e.zone], first: 'asc' },
  rating: { value: (e) => e.rating, first: 'desc' },
  start: { value: (e) => e.starting_rank, first: 'asc' },
};
