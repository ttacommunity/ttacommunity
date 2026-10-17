/** What became of an action in admin, where on the page it shows, and what a screen reader is told of it: the decisions behind Notice. */
import type { AdminResult } from './admin-api';

/** Outcome is an action done or refused, or a load that failed, with what to say; at names the place on the page it shows. */
export type Outcome = { kind: 'done' | 'refused' | 'failed'; text: string; at: string };

/** outcomeOf is what became of a request: done, saying done, or refused with the API's reason; shown at at, or a success at doneAt when it removed what at is. */
export function outcomeOf(r: AdminResult<unknown>, done: string, at: string, doneAt = at): Outcome {
  return r.ok ? { kind: 'done', text: done, at: doneAt } : { kind: 'refused', text: r.message, at };
}

/** refusalOf is a request's refusal shown at at, or null when it was done. */
export function refusalOf(r: AdminResult<unknown>, at = ''): Outcome | null {
  return r.ok ? null : { kind: 'refused', text: r.message, at };
}

/** succeeded is an action done, saying what was done. */
export function succeeded(text: string, at: string): Outcome {
  return { kind: 'done', text, at };
}

/** refused is an action refused before it is sent, as when a form is not filled in. */
export function refused(text: string, at: string): Outcome {
  return { kind: 'refused', text, at };
}

/** loadFailure is something a page or panel could not load, and why. */
export function loadFailure(text: string, at: string): Outcome {
  return { kind: 'failed', text, at };
}

/** shownAt is the outcome when it belongs at at, else null: each place on a page shows only its own. */
export function shownAt(o: Outcome | null, at: string): Outcome | null {
  return o !== null && o.at === at ? o : null;
}

const HEADINGS: Record<Outcome['kind'], string> = { done: 'Done', refused: 'Not done', failed: 'Could not load' };

/** noticeHeading is the word a notice starts with, so its kind never rests on colour alone. */
export function noticeHeading(kind: Outcome['kind']): string {
  return HEADINGS[kind];
}

/** revealed is whether a notice scrolls itself into view: an action's outcome does, beside the action; a load failure, which no one acted for, does not. */
export function revealed(o: Outcome): boolean {
  return o.kind !== 'failed';
}

/** announcement is what a screen reader is told of an outcome, and in which live region: a refusal or failure at once (alert), a success when the reader is free (status); null for none. */
export function announcement(o: Outcome | null): { region: 'alert' | 'status'; text: string } | null {
  if (o === null) return null;
  return { region: o.kind === 'done' ? 'status' : 'alert', text: `${noticeHeading(o.kind)}. ${o.text}` };
}
