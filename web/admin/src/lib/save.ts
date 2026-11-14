/** Saving from an admin box: the word beside its button while the request runs and after, and the refusal when there is one. */
import type { AdminResult } from './admin-api';
import { refusalOf, type Outcome } from './outcome';

/** SaveState is what a box shows of its last save: a word beside the button, a refusal under it, and at, the place on the box it is about ('' for the box). */
export type SaveState = { status: string; refusal: Outcome | null; at: string };

/** IDLE is a box before any save, or once its refusal is closed. */
export const IDLE: SaveState = { status: '', refusal: null, at: '' };

/** runSave sends (null: nothing to save, and nothing is shown), setting the working word, then done's word or the refusal; it returns the result, or null. */
export async function runSave<T>(
  send: (() => Promise<AdminResult<T>>) | null,
  { done, working = 'Saving…', at = '' }: { done: string | ((data: T) => string); working?: string; at?: string },
  set: (s: SaveState) => void,
): Promise<AdminResult<T> | null> {
  if (send === null) return null;
  set({ status: working, refusal: null, at });
  const r = await send();
  if (r.ok) set({ status: typeof done === 'string' ? done : done(r.data), refusal: null, at });
  else set({ status: '', refusal: refusalOf(r, at), at });
  return r;
}

/** statusText is the word beside a box's buttons: status after prefix (a count, say), either alone when the other is empty. */
export function statusText(prefix: string, status: string): string {
  if (prefix === '') return status;
  return status === '' ? prefix : `${prefix} · ${status}`;
}
