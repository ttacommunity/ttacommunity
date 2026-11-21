/** A player's report of their own blitz match: reading it, what the report box says game by game and about best of 3, the flag's question, the form's start, its row in the pairings, and the requests that report, confirm, flag and answer best of 3. */
import type { components } from '@ttac/shared/api';
import { bestOfScore, resultSummary, type BlitzPlayedGame, type BlitzResult } from '@ttac/shared/blitz';
import { emptyGameForm, type BlitzGameRecord, type GameForm } from '@ttac/shared/game-form';
import type { TextQuestion } from '@ttac/shared/Dialog';
import { clockIn } from '@ttac/shared/time';
import { AccountApiError, change, failureMessage, type Done } from './account';
import type { YourRow } from './blitz';

export type MyBlitzMatch = components['schemas']['MyBlitzMatch'];
export type MyBlitz = components['schemas']['MyBlitz'];
export type MyBlitzGame = components['schemas']['MyBlitzGame'];
export type BestOfAction = components['schemas']['BlitzBestOfAction'];
type GameAction = MyBlitzGame['actions'][number];

function url(base: string, path: string): string {
  return base.replace(/\/+$/, '') + path;
}

function matchPath(slug: string, edition: string, id: number, end: string): string {
  return `/v1/me/blitz/${encodeURIComponent(slug)}/${encodeURIComponent(edition)}/matches/${id}/${end}`;
}

/** fetchMyBlitz returns the signed-in player's match in the edition's latest round (null for none) and who reported each game (empty unless they are entered); null when nobody is signed in. */
export async function fetchMyBlitz(base: string, slug: string, edition: string, fetchFn: typeof fetch = fetch): Promise<MyBlitz | null> {
  const res = await fetchFn(url(base, `/v1/me/blitz/${encodeURIComponent(slug)}/${encodeURIComponent(edition)}`), { credentials: 'include' });
  if (res.status === 401) return null;
  if (!res.ok) throw new AccountApiError(res.status, await failureMessage(res));
  return (await res.json()) as MyBlitz;
}

/** answerBestOf proposes best of 3, accepts or declines the opponent's proposal, or withdraws the player's own. */
export function answerBestOf(base: string, slug: string, edition: string, id: number, answer: BestOfAction, fetchFn: typeof fetch = fetch): Promise<Done> {
  return change(fetchFn, base, 'POST', matchPath(slug, edition, id, 'best-of'), { answer });
}

/** reportGame reports game number of the match, or corrects the player's own report of it. */
export function reportGame(base: string, slug: string, edition: string, id: number, number: number, game: BlitzGameRecord, fetchFn: typeof fetch = fetch): Promise<Done> {
  return change(fetchFn, base, 'PUT', matchPath(slug, edition, id, `games/${number}`), game);
}

/** confirmReport confirms the opponent's report of game number. */
export function confirmReport(base: string, slug: string, edition: string, id: number, number: number, fetchFn: typeof fetch = fetch): Promise<Done> {
  return change(fetchFn, base, 'POST', matchPath(slug, edition, id, `games/${number}/confirm`));
}

/** flagReport flags the opponent's report of game number as wrong, saying why. */
export function flagReport(base: string, slug: string, edition: string, id: number, number: number, reason: string, fetchFn: typeof fetch = fetch): Promise<Done> {
  return change(fetchFn, base, 'POST', matchPath(slug, edition, id, `games/${number}/flag`), { reason: reason.trim() });
}

/** sides names the player and their opponent. */
function sides(m: { a: string; b: string | null; me: 'a' | 'b' }): { mine: string; theirs: string } {
  return m.me === 'a' ? { mine: m.a, theirs: m.b ?? '' } : { mine: m.b ?? '', theirs: m.a };
}

/** GameView is one game of the player's match as its line reads it: the sides, its result (the match's, for a one-game match), the game played and a player's report of it, and what the player may do with it. */
type GameView = {
  a: string;
  b: string | null;
  me: 'a' | 'b';
  result: BlitzResult | null;
  ruled: boolean;
  game: BlitzPlayedGame | null;
  report: MyBlitzGame['report'];
  reported_by: MyBlitzGame['reported_by'];
  flag_reason: string;
  actions: GameAction[];
};

/** gameStatus says where a game stands and what the player can do about it. */
function gameStatus(m: GameView): string {
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

/** ReportLine is one line of the report box: the game it is about (0 for the match), its heading ('' in a one-game match), what it says, what the player may do with it, and the game played (null for none yet). */
export type ReportLine = { number: number; heading: string; text: string; actions: GameAction[]; game: BlitzPlayedGame | null };

/** reportLines says where the player's match stands, game by game: one line for a one-game match (or a bye), as before best of 3; for a best of 3, a line per game played, one for the game to report next (or that the next round is paired), and the organisers' ruling if they decided it. */
export function reportLines(m: MyBlitzMatch): ReportLine[] {
  const unplayed = (actions: GameAction[]): GameView => ({ ...m, result: null, ruled: false, game: null, report: null, reported_by: null, flag_reason: '', actions });
  const ofGame = (g: MyBlitzGame): GameView => ({ ...m, ...g, ruled: false });
  const line = (number: number, heading: string, v: GameView): ReportLine => ({ number, heading, text: gameStatus(v), actions: v.actions, game: v.game });
  if (m.best_of === 1) {
    const g = m.games[0];
    const v = g === undefined ? { ...unplayed(m.next_game === 1 ? ['report'] : []), result: m.result, ruled: m.ruled } : { ...ofGame(g), result: m.result, ruled: m.ruled };
    return [line(1, '', v)];
  }
  const lines = m.games.map((g) => line(g.number, `Game ${g.number}`, ofGame(g)));
  if (m.next_game !== null) lines.push(line(m.next_game, `Game ${m.next_game}`, unplayed(['report'])));
  else if (m.result === null) lines.push(line(m.games.length + 1, `Game ${m.games.length + 1}`, unplayed([])));
  if (m.ruled) lines.push({ number: 0, heading: '', text: `An organiser decided the match: ${m.result === 'a' ? m.a : m.b} goes through.`, actions: [], game: null });
  return lines;
}

/** BEST_OF_LABELS name each best-of-3 answer's button. */
export const BEST_OF_LABELS: Record<BestOfAction, string> = { propose: 'Propose best of 3', withdraw: 'Withdraw proposal', accept: 'Accept best of 3', decline: 'Decline' };

/** bestOfStatus says where best of 3 stands for the player's match: what they may propose or answer, or the score once agreed and who goes through; '' when it is not offered or settled as one game. */
export function bestOfStatus(m: MyBlitzMatch): string {
  const { theirs } = sides(m);
  const rule = 'a win is 1 and a draw ½, the first to 2 goes through, and the higher seed if level after three';
  if (m.b === null) return '';
  if (m.best_of === 3) {
    const through = m.result !== null && !m.ruled ? ` ${m.result === 'a' ? m.a : m.b} goes through.` : '';
    return `Best of 3: ${bestOfScore(m)}.${through}`;
  }
  if (m.best_of_actions.includes('propose')) return `This round's matches may be best of 3 if you and ${theirs} both agree before game 1: ${rule}.`;
  if (m.best_of_actions.includes('withdraw')) return `You proposed best of 3. ${theirs} can accept it until game 1 is reported.`;
  if (m.best_of_actions.includes('accept')) return `${theirs} proposes best of 3: ${rule}. Accept it before game 1, or decline to play one game.`;
  return '';
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

/** formFor starts the report form: empty, today, in the player's zone; or, for a correction, the game reported, its finish on the player's clocks when the browser can read their zone. */
export function formFor(game: BlitzPlayedGame | null, zone: string, today: string): GameForm {
  const empty = emptyGameForm(today, zone);
  if (game === null) return empty;
  const at = clockIn(zone, game.finished_at);
  return {
    ...empty,
    share_code: game.share_code,
    a_culture: String(game.a_culture),
    a_outcome: game.a_outcome,
    a_turn: game.a_turn,
    b_culture: String(game.b_culture),
    b_outcome: game.b_outcome,
    b_turn: game.b_turn,
    ...(at === null ? {} : { finished_on: at.day, finished_time: at.clock }),
  };
}

/** yourRow marks the player's game in the pairings, saying what they may do with it; null for no game. */
export function yourRow(m: MyBlitzMatch | null): YourRow | null {
  if (m === null) return null;
  const actions = m.games.flatMap((g) => g.actions);
  const label = actions.includes('confirm')
    ? 'Your game: confirm or flag it'
    : m.best_of_actions.includes('accept')
      ? 'Your game: answer the best-of-3 proposal'
      : m.next_game !== null
        ? 'Your game: report it'
        : actions.includes('report')
          ? 'Your game: correct it'
          : 'Your game';
  return { round: m.round, sideA: m.a, label };
}
