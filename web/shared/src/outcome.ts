/** What an action came to, as both sites show it: done, refused, or a load that failed, each named in words. */

/** OutcomeKind is how an action ended. */
export type OutcomeKind = 'done' | 'refused' | 'failed';

const HEADINGS: Record<OutcomeKind, string> = { done: 'Done', refused: 'Not done', failed: 'Could not load' };

/** noticeHeading is the word a notice starts with, so its kind never rests on colour alone. */
export function noticeHeading(kind: OutcomeKind): string {
  return HEADINGS[kind];
}
