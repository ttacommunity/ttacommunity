/** Which of a page's sub tabs to show. */

/** shownTab is the tab to show: the one chosen while it is still offered, else the first offered. */
export function shownTab<T>(offered: readonly T[], chosen: T | null): T {
  return chosen !== null && offered.includes(chosen) ? chosen : offered[0];
}
