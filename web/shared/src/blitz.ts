/** What both sites say about a blitz tournament: a zone's offset, a format, the zones a select offers, and how a match's result reads. */
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

/** resultSummary says how a match ended: "bye", "" while undecided, else who won (or a draw, or a double loss), the score, and "by ruling" when the organisers decided it. */
export function resultSummary(m: MatchResultView): string {
  if (m.b === null) return 'bye';
  if (m.result === null) return '';
  const who = { a: `${m.a} won`, b: `${m.b} won`, draw: 'Draw', double_loss: 'Double loss' }[m.result];
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

/** formatLabel describes an edition's format in words. */
export function formatLabel(f: BlitzFormat): string {
  const swiss = `${f.swiss_rounds} Swiss round${f.swiss_rounds === 1 ? '' : 's'}`;
  return f.knockout_size === 0 ? `${swiss}, no knockout` : `${swiss}, then a top-${f.knockout_size} knockout`;
}

/** zoneOptions lists the zones a select offers: UTC first, then known in order, with current added when missing. */
export function zoneOptions(known: readonly string[], current: string): string[] {
  const rest = known.filter((z) => z !== 'UTC');
  if (current !== '' && current !== 'UTC' && !rest.includes(current)) rest.push(current);
  return ['UTC', ...rest.sort()];
}
