/** A player's report of their own blitz match: reading it, what the report box says, the flag's question, the form's start, its row in the pairings, and the requests that report, confirm and flag. */
import type { components } from '@ttac/shared/api';
import { resultSummary } from '@ttac/shared/blitz';
import { emptyGameForm, type BlitzGameRecord, type GameForm } from '@ttac/shared/game-form';
import type { TextQuestion } from '@ttac/shared/Dialog';
import { AccountApiError, change, failureMessage, type Done } from './account';
import type { YourRow } from './blitz';

export type MyBlitzMatch = components['schemas']['MyBlitzMatch'];

function url(base: string, path: string): string {
  return base.replace(/\/+$/, '') + path;
}

function matchPath(slug: string, edition: string, id: number, end: string): string {
  return `/v1/me/blitz/${encodeURIComponent(slug)}/${encodeURIComponent(edition)}/matches/${id}/${end}`;
}

/** fetchMyMatch returns the signed-in player's match in the edition's latest round, or null for none or nobody signed in. */
export async function fetchMyMatch(base: string, slug: string, edition: string, fetchFn: typeof fetch = fetch): Promise<MyBlitzMatch | null> {
  const res = await fetchFn(url(base, `/v1/me/blitz/${encodeURIComponent(slug)}/${encodeURIComponent(edition)}`), { credentials: 'include' });
  if (res.status === 401) return null;
  if (!res.ok) throw new AccountApiError(res.status, await failureMessage(res));
  return ((await res.json()) as { match: MyBlitzMatch | null }).match;
}

/** reportGame reports the match's game, or corrects the player's own report. */
export function reportGame(base: string, slug: string, edition: string, id: number, game: BlitzGameRecord, fetchFn: typeof fetch = fetch): Promise<Done> {
  return change(fetchFn, base, 'PUT', matchPath(slug, edition, id, 'game'), game);
}

/** confirmReport confirms the opponent's report. */
export function confirmReport(base: string, slug: string, edition: string, id: number, fetchFn: typeof fetch = fetch): Promise<Done> {
  return change(fetchFn, base, 'POST', matchPath(slug, edition, id, 'confirm'));
}

/** flagReport flags the opponent's report as wrong, saying why. */
export function flagReport(base: string, slug: string, edition: string, id: number, reason: string, fetchFn: typeof fetch = fetch): Promise<Done> {
  return change(fetchFn, base, 'POST', matchPath(slug, edition, id, 'flag'), { reason: reason.trim() });
}

/** sides names the player and their opponent. */
function sides(m: MyBlitzMatch): { mine: string; theirs: string } {
  return m.me === 'a' ? { mine: m.a, theirs: m.b ?? '' } : { mine: m.b ?? '', theirs: m.a };
}

/** myMatchStatus says where the player's match stands and what they can do about it. */
export function myMatchStatus(m: MyBlitzMatch): string {
  const { theirs } = sides(m);
  const result = resultSummary(m);
  const mineReported = m.reported_by === m.me;
  if (m.b === null) return 'You have the bye this round.';
  if (m.result === null) {
    return m.actions.includes('report')
      ? 'Not reported yet. Report your game once it is played: it counts at once, and your opponent can confirm or flag it.'
      : 'The next round is paired: ask an organiser to record this game.';
  }
  switch (m.report) {
    case 'reported':
      return mineReported
        ? `You reported ${result}. ${theirs} can confirm or flag it; until then you can correct it.`
        : `${theirs} reported ${result}. Confirm it if it is right, or flag it if not.`;
    case 'confirmed': {
      const reporter = m.reported_by === 'a' ? m.a : (m.b ?? '');
      const confirmer = m.reported_by === 'a' ? (m.b ?? '') : m.a;
      return `${reporter} reported ${result}, and ${confirmer} confirmed it. Ask an organiser if anything is wrong.`;
    }
    case 'flagged':
      return `${mineReported ? `${theirs} flagged your report` : 'You flagged this report'}: “${m.flag_reason}”. An organiser will resolve it.`;
    case 'resolved':
      return `An organiser resolved this result: ${result}.`;
  }
  return `An organiser recorded this result: ${result}.`;
}

/** flagQuestion asks what is wrong with the opponent's report, suggesting they be asked to correct it first. */
export function flagQuestion(m: MyBlitzMatch): TextQuestion {
  const { theirs } = sides(m);
  return {
    heading: `Flag ${theirs}'s report?`,
    text: `If the result is wrong, ask ${theirs} on Discord to correct it first: they can until you confirm or flag it. A flag locks the result until an organiser resolves it.`,
    action: 'Flag report',
    field: 'What is wrong',
    required: true,
    maxLength: 500,
  };
}

/** clockIn reads an instant on an IANA zone's clocks as a day and a time, or null for a zone the browser cannot read (a region, Unknown). */
function clockIn(zone: string, instant: string): { day: string; clock: string } | null {
  let format: Intl.DateTimeFormat;
  try {
    format = new Intl.DateTimeFormat('en-CA', { timeZone: zone, year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hourCycle: 'h23' });
  } catch {
    return null;
  }
  const parts = Object.fromEntries(format.formatToParts(new Date(instant)).map((p) => [p.type, p.value]));
  return { day: `${parts.year}-${parts.month}-${parts.day}`, clock: `${parts.hour}:${parts.minute}` };
}

/** formFor starts the report form: empty, today, in the player's zone; or, for a correction, the game reported, its finish on the player's clocks when the browser can read their zone. */
export function formFor(m: MyBlitzMatch, zone: string, today: string): GameForm {
  const empty = emptyGameForm(today, zone);
  if (m.game === null) return empty;
  const at = clockIn(zone, m.game.finished_at);
  return {
    ...empty,
    share_code: m.game.share_code,
    a_culture: String(m.game.a_culture),
    a_outcome: m.game.a_outcome,
    b_culture: String(m.game.b_culture),
    b_outcome: m.game.b_outcome,
    ...(at === null ? {} : { finished_on: at.day, finished_time: at.clock }),
  };
}

/** yourRow marks the player's game in the pairings, saying what they may do with it; null for no game. */
export function yourRow(m: MyBlitzMatch | null): YourRow | null {
  if (m === null) return null;
  const label = m.actions.includes('confirm')
    ? 'Your game: confirm or flag it'
    : m.actions.includes('report')
      ? m.reported_by === m.me
        ? 'Your game: correct it'
        : 'Your game: report it'
      : 'Your game';
  return { round: m.round, a: m.a, label };
}
