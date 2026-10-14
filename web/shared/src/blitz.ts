/** What both sites say about a blitz tournament: a zone's offset, a format, how a match's result reads, how a folded round is summed up, and how a round and a seeded player are named. */
import type { components } from '../api/schema';

export type BlitzFormat = components['schemas']['BlitzFormat'];
export type BlitzResult = components['schemas']['BlitzResult'];
export type BlitzPlayedGame = components['schemas']['BlitzPlayedGame'];

/** MatchResultView is what a match's result line needs: both names (b null for the bye), the result, the game played and whether it was ruled. */
export type MatchResultView = { a: string; b: string | null; result: BlitzResult | null; game: BlitzPlayedGame | null; ruled: boolean };

/** scoreText writes a game's cultures as "250–200", naming a player who resigned or timed out. */
function scoreText(m: MatchResultView, game: BlitzPlayedGame): string {
  const score = `${game.a_culture}–${game.b_culture}`;
  const ended = (name: string | null, outcome: BlitzPlayedGame['a_outcome']) =>
    outcome === 'finished' ? '' : `, ${name ?? '?'} ${outcome === 'resigned' ? 'resigned' : 'timed out'}`;
  return score + ended(m.a, game.a_outcome) + ended(m.b, game.b_outcome);
}

/** drawnGame says whether a game ended level: both players finished on the same culture. */
function drawnGame(g: BlitzPlayedGame): boolean {
  return g.a_outcome === 'finished' && g.b_outcome === 'finished' && g.a_culture === g.b_culture;
}

/** resultSummary says how a match ended: "bye", "" while undecided, else who won (or a draw, or a double loss), the score, "by ruling" when the organisers decided it, and who went through as the higher seed when a knockout game was drawn. */
export function resultSummary(m: MatchResultView): string {
  if (m.b === null) return 'bye';
  if (m.result === null) return '';
  const who = { a: `${m.a} won`, b: `${m.b} won`, draw: 'Draw', double_loss: 'Double loss' }[m.result];
  if (!m.ruled && m.game !== null && drawnGame(m.game) && (m.result === 'a' || m.result === 'b')) {
    return `Draw ${scoreText(m, m.game)}: ${m.result === 'a' ? m.a : m.b} through as the higher seed`;
  }
  if (!m.ruled) return m.game === null ? who : `${who} ${scoreText(m, m.game)}`;
  return m.game === null ? `${who} by ruling` : `${who} by ruling (game ${scoreText(m, m.game)})`;
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
