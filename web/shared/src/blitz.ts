/** What both sites say about a blitz tournament: a zone's offset, a format, its points, how a match's result and turn order read, how a folded round is summed up, how a round and a seeded player are named, how the standings share places, and how rounds are paired. */
import type { components } from '../api/schema';
import type { SortColumns } from './sort';

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
export const FIRST_METHODS: readonly PairingMethod[] = ['dutch', 'dutch-timezone', 'timezone', 'random'];
/** LATER_METHODS are the methods the rounds after round 1 may be paired by, in the order offered. */
export const LATER_METHODS: readonly PairingMethod[] = ['dutch', 'dutch-timezone', 'timezone', 'rating'];

/** PAIRING_METHODS names each pairing method and says in plain words how it pairs a round. */
export const PAIRING_METHODS: Record<PairingMethod, { label: string; about: string }> = {
  dutch: {
    label: "FIDE's Dutch system",
    about:
      'Players on the same points meet. Each score group is ranked by rating and split in two, and the top half plays the bottom half: the first against the first of the bottom half, the second against the second, and so on. Pairs change only to avoid a rematch, and a player left over moves down to the next group.',
  },
  'dutch-timezone': {
    label: 'Dutch, time zones as tiebreak',
    about: "As FIDE's Dutch system, the top half of each score group plays the bottom half; within that, each player meets the opponent in the bottom half closest to their time zone.",
  },
  timezone: { label: 'Closest time zones', about: 'Players on the same points meet the opponent closest to their time zone, then the closest in rating.' },
  rating: { label: 'Closest ratings', about: 'Players on the same points meet the opponent closest in rating.' },
  random: { label: 'Random', about: 'Opponents are drawn by chance.' },
};

/** pairingSummary says in one line how an edition's rounds are paired: one method for every round, or round 1's and the later rounds'. */
export function pairingSummary(p: PairingMethods): string {
  if (p.first === p.later) return `${PAIRING_METHODS[p.first].label}, every round`;
  return `Round 1: ${PAIRING_METHODS[p.first].label}; later rounds: ${PAIRING_METHODS[p.later].label}`;
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

/** placeLabel writes a standing's place: "3" when held alone, "1–12" when shared. */
export function placeLabel(s: { place_from: number; place_to: number }): string {
  return s.place_from === s.place_to ? String(s.place_from) : `${s.place_from}–${s.place_to}`;
}

/** FinishRound is what latestFinishes reads of a round: its number, kind, and each match's players and game. */
export type FinishRound = { number: number; kind: 'swiss' | 'knockout'; matches: readonly { a: string; b: string | null; game: { finished_at: string } | null }[] };

/** latestFinishes maps each player with a game in the latest Swiss round that has one to when that game finished. */
export function latestFinishes(rounds: readonly FinishRound[]): Map<string, string> {
  const latest = rounds
    .filter((r) => r.kind === 'swiss' && r.matches.some((m) => m.game !== null))
    .reduce<FinishRound | null>((best, r) => (best === null || r.number > best.number ? r : best), null);
  const out = new Map<string, string>();
  for (const m of latest?.matches ?? []) {
    if (m.game === null || m.b === null) continue;
    out.set(m.a, m.game.finished_at);
    out.set(m.b, m.game.finished_at);
  }
  return out;
}

/** byFinish orders the standings within each shared place by when each player's game finished (finished, from latestFinishes), earliest first; those without one follow in the order given. */
export function byFinish<T extends { name: string; place_from: number }>(standings: readonly T[], finished: ReadonlyMap<string, string>): T[] {
  const at = (s: T) => finished.get(s.name) ?? null;
  return standings
    .map((s, i) => ({ s, i }))
    .sort((x, y) => {
      if (x.s.place_from !== y.s.place_from) return x.i - y.i;
      const a = at(x.s);
      const b = at(y.s);
      if (a === null || b === null) return a === b ? x.i - y.i : a === null ? 1 : -1;
      return a === b ? x.i - y.i : a < b ? -1 : 1;
    })
    .map(({ s }) => s);
}

export type BlitzStanding = components['schemas']['BlitzStanding'];

/** STANDING_COLUMNS are the standings tables' sortable columns, on the site and in admin. */
export const STANDING_COLUMNS: SortColumns<BlitzStanding, 'place' | 'player' | 'points' | 'played' | 'byes' | 'buchholz'> = {
  place: { value: (s) => s.place_from, first: 'asc' },
  player: { value: (s) => s.name, first: 'asc' },
  points: { value: (s) => s.points, first: 'desc' },
  played: { value: (s) => s.games, first: 'desc' },
  byes: { value: (s) => s.byes, first: 'desc' },
  buchholz: { value: (s) => s.buchholz, first: 'desc' },
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
export type EntrantRow = { name: string; zone: string; utc_offset: string; rating: number | null };

/** ENTRANT_COLUMNS are an entrants table's sortable columns; zone sorts west to east, then by the zone's name. */
export const ENTRANT_COLUMNS: SortColumns<EntrantRow, 'player' | 'zone' | 'rating'> = {
  player: { value: (e) => e.name, first: 'asc' },
  zone: { value: (e) => [offsetMinutes(e.utc_offset), e.zone], first: 'asc' },
  rating: { value: (e) => e.rating, first: 'desc' },
};
