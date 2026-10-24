/** Which of a page's sub tabs to show, and which one an address names. */

/** shownTab is the tab to show: the one chosen while it is still offered, else the first offered. */
export function shownTab<T>(offered: readonly T[], chosen: T | null): T {
  return chosen !== null && offered.includes(chosen) ? chosen : offered[0];
}

/** hashTab is the offered tab an address's hash names ("#pairings"), or null for none. */
export function hashTab<T extends string>(offered: readonly T[], hash: string): T | null {
  return offered.find((t) => `#${t}` === hash) ?? null;
}
