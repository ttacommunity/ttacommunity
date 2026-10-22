/** What both sites say about a blitz tournament: a zone's offset, a format, its points, how a match's result and turn order read, how a folded round is summed up, and how a round and a seeded player are named. */
import type { components } from '../api/schema';

export type BlitzFormat = components['schemas']['BlitzFormat'];
export type BlitzResult = components['schemas']['BlitzResult'];
export type BlitzPlayedGame = components['schemas']['BlitzPlayedGame'];
export type BlitzPoints = components['schemas']['BlitzPoints'];

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

/** resultSummary says how a match ended: "bye", "" while undecided, else who won (or a draw, or a double loss), the score, "by ruling" when the organisers overruled a game, "by forfeit" when they decided it with no game played, and who went through as the higher seed when a knockout game was drawn. */
export function resultSummary(m: MatchResultView): string {
  if (m.b === null) return 'bye';
  if (m.result === null) return '';
  const who = { a: `${m.a} won`, b: `${m.b} won`, draw: 'Draw', double_loss: 'Double loss' }[m.result];
  if (!m.ruled && m.game !== null && drawnGame(m.game) && (m.result === 'a' || m.result === 'b')) {
    return `Draw ${scoreText(m, m.game)}: ${m.result === 'a' ? m.a : m.b} through as the higher seed`;
  }
  if (!m.ruled) return m.game === null ? who : `${who} ${scoreText(m, m.game)}`;
  if (m.game !== null) return `${who} by ruling (game ${scoreText(m, m.game)})`;
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
