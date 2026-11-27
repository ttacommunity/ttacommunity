/** Admin's game changes: turning the add, remove and replace form into the request, and the changes into the table's rows. The API checks the game itself. */
import type { components } from '@ttac/shared/api';

type Schemas = components['schemas'];
export type GameChange = Schemas['GameChange'];
export type ChangeableGame = Schemas['ChangeableGame'];
export type SeasonStage = Schemas['SeasonStage'];
export type GameChangeRequest = Schemas['GameChangeRequest'];
export type Action = Schemas['GameChangeAction'];
export type SeatOutcome = Schemas['SeatOutcome'];

/** ACTIONS are the changes admin makes, in the order offered. */
export const ACTIONS: { value: Action; label: string }[] = [
  { value: 'add', label: 'Add' },
  { value: 'remove', label: 'Remove' },
  { value: 'replace', label: 'Replace' },
];

/** OUTCOMES are how a seat's game ended, as offered. */
export const OUTCOMES: readonly { value: SeatOutcome; label: string }[] = [
  { value: 'finished', label: 'finished' },
  { value: 'resigned', label: 'resigned' },
  { value: 'timed_out', label: 'timed out' },
];

/** SeatForm is one seat as typed: place and culture as text. */
export type SeatForm = { player: string; place: string; outcome: SeatOutcome; culture: string };

/** EntryForm is the new game as typed: its replay code, day (YYYY-MM-DD), number in its group ("" for the next) and 2 to 4 seats. */
export type EntryForm = { code: string; date: string; number: string; seats: SeatForm[] };

const MIN_SEATS = 2;
const MAX_SEATS = 4;

const seat = (place: number): SeatForm => ({ player: '', place: String(place), outcome: 'finished', culture: '' });

/** emptyEntry is a new game before anything is typed: two seats, first and second. */
export function emptyEntry(): EntryForm {
  return { code: '', date: '', number: '', seats: [seat(1), seat(2)] };
}

/** entryFrom starts a replacement from the game it replaces: its day and players, places, outcomes and culture, and no code. */
export function entryFrom(g: ChangeableGame): EntryForm {
  return {
    code: '', date: g.date, number: '',
    seats: g.seats.map((s) => ({ player: s.player, place: String(s.place), outcome: s.outcome ?? 'finished', culture: String(s.score) })),
  };
}

/** withSeat adds a seat at the next place, up to four. */
export function withSeat(e: EntryForm): EntryForm {
  return e.seats.length >= MAX_SEATS ? e : { ...e, seats: [...e.seats, seat(e.seats.length + 1)] };
}

/** withoutSeat takes away the seat at index i, keeping two. */
export function withoutSeat(e: EntryForm, i: number): EntryForm {
  return e.seats.length <= MIN_SEATS ? e : { ...e, seats: e.seats.filter((_, j) => j !== i) };
}

/** Picked is what the form holds: the game to remove or replace, the group to add to, the new game and the reason. */
export type Picked = { game?: number; group?: number; entry: EntryForm; reason: string };

type Body = { ok: true; body: GameChangeRequest } | { ok: false; message: string };

/** changeBody turns the form into the request for action, or says what is missing: a reason, the game or group, a finished player's culture, a whole game number. */
export function changeBody(action: Action, p: Picked): Body {
  const reason = p.reason.trim();
  if (reason === '') return { ok: false, message: 'Say why: the reason is kept in admin.' };
  if (action === 'add' && p.group === undefined) return { ok: false, message: 'Choose the group to add the game to.' };
  if (action !== 'add' && p.game === undefined) return { ok: false, message: 'Find the game first.' };
  if (action === 'remove') return { ok: true, body: { action, game: p.game, reason } };
  const entry = entryBody(p.entry, action === 'add');
  if (!entry.ok) return entry;
  return { ok: true, body: action === 'add' ? { action, group: p.group, entry: entry.body, reason } : { action, game: p.game, entry: entry.body, reason } };
}

/** entryBody turns the typed game into the API's, its number kept only for an addition. */
function entryBody(e: EntryForm, numbered: boolean): { ok: true; body: Schemas['GameEntry'] } | { ok: false; message: string } {
  const seats: Schemas['GameEntrySeat'][] = [];
  for (const s of e.seats) {
    const score = s.outcome === 'finished' ? (s.culture.trim() === '' ? NaN : Number(s.culture)) : 0;
    if (!Number.isFinite(score)) return { ok: false, message: 'A finished player needs their culture, as a number.' };
    seats.push({ player: s.player.trim(), place: Number(s.place), outcome: s.outcome, score });
  }
  const body: Schemas['GameEntry'] = { code: e.code.trim(), date: e.date, seats };
  if (numbered && e.number.trim() !== '') {
    const n = Number(e.number);
    if (!Number.isInteger(n) || n < 1) return { ok: false, message: 'The game number is a whole number from 1, or empty for the next.' };
    body.number = n;
  }
  return { ok: true, body };
}

const spaced = (s: string) => s.replace('_', ' ');

type Named = { name: string; code: string };

/** gameLabel names a game by name and replay code, "" for none. */
function gameLabel(g: Named | null): string {
  return g === null ? '' : `${g.name} (${g.code === '' ? 'no code' : g.code})`;
}

/** ChangeRow is one change as the table shows it, with what deleting it does, and the change itself. */
export type ChangeRow = { id: number; when: string; action: string; game: string; added: string; reason: string; by: string; undo: string; change: GameChange };

/** changeRows are the changes as the table shows them, in the order given; when writes an instant. */
export function changeRows(changes: readonly GameChange[], when: (iso: string) => string): ChangeRow[] {
  return changes.map((c) => ({
    id: c.id, when: when(c.at), action: ACTIONS.find((a) => a.value === c.action)?.label ?? c.action,
    game: gameLabel(c.game), added: gameLabel(c.added), reason: c.reason, by: c.by === '' ? 'not known' : c.by, undo: undoText(c.action), change: c,
  }));
}

/** undoText says what undoing a change of the action does. */
export function undoText(action: Action): string {
  switch (action) {
    case 'add':
      return 'The game added is deleted.';
    case 'remove':
      return 'The game gets back its earlier state.';
    case 'replace':
      return 'The new game is deleted, and the game it replaced gets back its earlier state.';
  }
}

/** groupChoices are a season's groups to add a game to, named by stage and group, with the number a game added gets. */
export function groupChoices(stages: readonly SeasonStage[]): { id: number; label: string; next: number }[] {
  return stages.flatMap((st) =>
    st.groups.map((g) => ({
      id: g.id, label: `${[st.label, g.name].filter((x) => x !== '').join(' - ')} (${g.games} games)`, next: g.next_number,
    })),
  );
}


/** foundLine says what a game found is: its day, state, and each player in place order with how they ended and their culture. */
export function foundLine(g: ChangeableGame): string {
  const seats = g.seats.map((s) => `${s.place}. ${s.player} (${s.outcome === null ? 'not known' : spaced(s.outcome)}, ${s.score})`).join('; ');
  return `${g.date}, ${spaced(g.state)}: ${seats === '' ? 'no players' : seats}`;
}

/** PickedGroup is the group a game is added to: its id, the number a game added gets, and its name where known (an edit's). */
export type PickedGroup = { id: number; next: number; label?: string };

/** ChangeForm is the change sheet's form: the action, the game found to remove or replace, the group to add to, the new game and the reason. */
export type ChangeForm = { action: Action; found: ChangeableGame | null; group: PickedGroup | null; entry: EntryForm; reason: string };

/** editStart is the form for editing change c, given its original and added games as they are now (null for none): the new game as it is, code and number kept, or one ready from the original's players. */
export function editStart(c: GameChange, original: ChangeableGame | null, added: ChangeableGame | null): ChangeForm {
  const entry = added !== null ? { ...entryFrom(added), code: added.code, number: added.number === null ? '' : String(added.number) } : original !== null ? entryFrom(original) : emptyEntry();
  const group = c.action === 'add' && added !== null && added.group !== null
    ? { id: added.group, next: added.number ?? 1, label: added.name.replace(/ game \d+$/, '') }
    : null;
  return { action: c.action, found: original, group, entry, reason: c.reason };
}
