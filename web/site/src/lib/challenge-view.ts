/** The challenge pages' switches a browser remembers between visits: which entries count, how players compare, and the highscores' view. */

const CHOICES = {
  verified: ['verified', 'all'],
  measure: ['total', 'rating'],
  view: ['cards', 'compact'],
} as const;

/** Choice is a remembered switch; each one's first value is its default. */
export type Choice = keyof typeof CHOICES;
export type ChoiceValue<C extends Choice> = (typeof CHOICES)[C][number];

/** recallChoice returns the remembered value of choice if it is still offered, else its default (also without a store). */
export function recallChoice<C extends Choice>(store: Storage | null, choice: C): ChoiceValue<C> {
  const values: readonly string[] = CHOICES[choice];
  let saved: string | null = null;
  try {
    saved = store?.getItem(`ttac.challenges.${choice}`) ?? null;
  } catch {
    // Private windows and blocked storage refuse; the page works without memory.
  }
  return (saved !== null && values.includes(saved) ? saved : values[0]) as ChoiceValue<C>;
}

/** rememberChoice saves value for choice; a refusing store, or none, is ignored. */
export function rememberChoice<C extends Choice>(store: Storage | null, choice: C, value: ChoiceValue<C>): void {
  try {
    store?.setItem(`ttac.challenges.${choice}`, value);
  } catch {
    // As in recallChoice.
  }
}

type LoadLike<T> = { kind: 'loading' } | { kind: 'error'; message: string } | { kind: 'ready'; data: T };

/** shownLoad is what a page shows of next: while it loads again, the last ready data, so the page keeps its height and scroll. */
export function shownLoad<T>(last: LoadLike<T> | null, next: LoadLike<T>): LoadLike<T> {
  return next.kind === 'loading' && last !== null && last.kind === 'ready' ? last : next;
}
