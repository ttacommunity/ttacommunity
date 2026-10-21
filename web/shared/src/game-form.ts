/** The game form's decisions, for organisers and players alike: turning what was typed into the game the API records, or the reason it cannot. */
import type { components } from '@ttac/shared/api';

type Schemas = components['schemas'];
export type BlitzGameRecord = Schemas['BlitzGameRecord'];
export type BlitzOutcome = Schemas['BlitzOutcome'];
export type BlitzStanding = Schemas['BlitzStanding'];

/** GameForm is the result form as typed: cultures as text, outcomes chosen, and when it finished: the day (YYYY-MM-DD) and time (HH:MM) on the clocks of a zone (IANA, a region or Unknown). */
export type GameForm = {
  share_code: string;
  a_culture: string;
  a_outcome: BlitzOutcome;
  b_culture: string;
  b_outcome: BlitzOutcome;
  finished_on: string;
  finished_time: string;
  zone: string;
};

/** emptyGameForm is the form before anything is typed: both finished, today, no time yet, in the zone given. */
export function emptyGameForm(today: string, zone: string): GameForm {
  return { share_code: '', a_culture: '', a_outcome: 'finished', b_culture: '', b_outcome: 'finished', finished_on: today, finished_time: '', zone };
}

/** culture reads a typed culture: 0 for a player who did not finish, else a number of 0 or more, or null when it is not one. */
function culture(text: string, outcome: BlitzOutcome): number | null {
  if (outcome !== 'finished') return 0;
  const n = text.trim() === '' ? NaN : Number(text);
  return Number.isFinite(n) && n >= 0 ? n : null;
}

/** gameRecordFrom turns the form into the request body (the share code may be left empty unless codeRequired, as for a player's report), or says what is missing: a culture for each finished player, the day, time and zone it finished, and at least one player who finished. */
export function gameRecordFrom(f: GameForm, { codeRequired = false } = {}): { ok: true; body: BlitzGameRecord } | { ok: false; message: string } {
  const share_code = f.share_code.trim();
  if (codeRequired && share_code === '') return { ok: false, message: "The game's share code is needed." };
  const a = culture(f.a_culture, f.a_outcome);
  const b = culture(f.b_culture, f.b_outcome);
  if (a === null || b === null) return { ok: false, message: 'A finished player needs a culture of 0 or more.' };
  if (f.finished_on === '') return { ok: false, message: 'The day it finished is needed.' };
  if (f.finished_time === '') return { ok: false, message: 'The time it finished is needed.' };
  if (f.zone.trim() === '') return { ok: false, message: 'The time zone of that time is needed.' };
  if (f.a_outcome !== 'finished' && f.b_outcome !== 'finished') return { ok: false, message: 'At least one player finished; record a ruling otherwise.' };
  return { ok: true, body: { share_code, a: { culture: a, outcome: f.a_outcome }, b: { culture: b, outcome: f.b_outcome }, finished_on: f.finished_on, finished_time: f.finished_time, zone: f.zone.trim() } };
}
