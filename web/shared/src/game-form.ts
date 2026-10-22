/** The game form's decisions, for organisers and players alike: turning what was typed into the game the API records, or the reason it cannot. */
import type { components } from '@ttac/shared/api';

type Schemas = components['schemas'];
export type BlitzGameRecord = Schemas['BlitzGameRecord'];
export type BlitzOutcome = Schemas['BlitzOutcome'];
export type BlitzStanding = Schemas['BlitzStanding'];

/** GameForm is the result form as typed: cultures as text, outcomes and turns chosen (1 for who went first, null for none yet), and when it finished: the day (YYYY-MM-DD) and time (HH:MM) on the clocks of a zone (IANA, a region or Unknown). */
export type GameForm = {
  share_code: string;
  a_culture: string;
  a_outcome: BlitzOutcome;
  a_turn: number | null;
  b_culture: string;
  b_outcome: BlitzOutcome;
  b_turn: number | null;
  finished_on: string;
  finished_time: string;
  zone: string;
};

/** A side of the game, as the match names them. */
export type GameSide = 'a' | 'b';

const SIDES: readonly GameSide[] = ['a', 'b'];

/** TURNS are the turns a game's players take, first first. */
export const TURNS: readonly number[] = [1, 2];

/** emptyGameForm is the form before anything is typed: both finished, no turns, today, no time yet, in the zone given. */
export function emptyGameForm(today: string, zone: string): GameForm {
  return { share_code: '', a_culture: '', a_outcome: 'finished', a_turn: null, b_culture: '', b_outcome: 'finished', b_turn: null, finished_on: today, finished_time: '', zone };
}

/** chooseTurn gives who the turn chosen: a player who held it takes who's old turn, and the last player without one takes the turn left. */
export function chooseTurn(f: GameForm, who: GameSide, turn: number): GameForm {
  const next: GameForm = { ...f, [`${who}_turn`]: turn };
  for (const s of SIDES) {
    if (s !== who && f[`${s}_turn`] === turn) next[`${s}_turn`] = f[`${who}_turn`];
  }
  const open = SIDES.filter((s) => next[`${s}_turn`] === null);
  const left = TURNS.filter((t) => !SIDES.some((s) => next[`${s}_turn`] === t));
  if (open.length === 1 && left.length === 1) next[`${open[0]}_turn`] = left[0];
  return next;
}

/** culture reads a typed culture: 0 for a player who did not finish, else a number of 0 or more, or null when it is not one. */
function culture(text: string, outcome: BlitzOutcome): number | null {
  if (outcome !== 'finished') return 0;
  const n = text.trim() === '' ? NaN : Number(text);
  return Number.isFinite(n) && n >= 0 ? n : null;
}

/** gameRecordFrom turns the form into the request body (the share code may be left empty unless codeRequired, as for a player's report), or says what is missing: a culture for each finished player, each player's turn, the day, time and zone it finished, and at least one player who finished. */
export function gameRecordFrom(f: GameForm, { codeRequired = false } = {}): { ok: true; body: BlitzGameRecord } | { ok: false; message: string } {
  const share_code = f.share_code.trim();
  if (codeRequired && share_code === '') return { ok: false, message: "The game's share code is needed." };
  const a = culture(f.a_culture, f.a_outcome);
  const b = culture(f.b_culture, f.b_outcome);
  if (a === null || b === null) return { ok: false, message: 'A finished player needs a culture of 0 or more.' };
  const turns = [f.a_turn, f.b_turn];
  if (f.a_turn === null || f.b_turn === null || !TURNS.every((t) => turns.includes(t))) return { ok: false, message: "Each player's turn is needed: who was P1, who P2." };
  if (f.finished_on === '') return { ok: false, message: 'The day it finished is needed.' };
  if (f.finished_time === '') return { ok: false, message: 'The time it finished is needed.' };
  if (f.zone.trim() === '') return { ok: false, message: 'The time zone of that time is needed.' };
  if (f.a_outcome !== 'finished' && f.b_outcome !== 'finished') return { ok: false, message: 'At least one player finished; record a ruling otherwise.' };
  return {
    ok: true,
    body: { share_code, a: { culture: a, outcome: f.a_outcome, turn: f.a_turn }, b: { culture: b, outcome: f.b_outcome, turn: f.b_turn }, finished_on: f.finished_on, finished_time: f.finished_time, zone: f.zone.trim() },
  };
}
