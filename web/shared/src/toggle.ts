/** Turning one item of a choice on or off, without changing the choice given. */

/** toggled returns list with item removed if it is there, else added at the end. */
export function toggled<T>(list: readonly T[], item: T): T[] {
  return list.includes(item) ? list.filter((x) => x !== item) : [...list, item];
}

/** toggledSet returns a copy of items with item added, or removed if it was there. */
export function toggledSet<T>(items: ReadonlySet<T>, item: T): Set<T> {
  const next = new Set(items);
  if (!next.delete(item)) next.add(item);
  return next;
}

/** toggledAll is an "All" button's choice: every value of every, or none when every one is already chosen. */
export function toggledAll<T>(chosen: ReadonlySet<T>, every: readonly T[]): Set<T> {
  return every.every((v) => chosen.has(v)) ? new Set() : new Set(every);
}

/** allButton is an "All" button's state: whether it is pressed, and the choice it makes; where none means all, it clears the choice. */
export function allButton<T>(chosen: ReadonlySet<T>, every: readonly T[], noneMeansAll: boolean): { pressed: boolean; next: Set<T> } {
  if (noneMeansAll) return { pressed: chosen.size === 0, next: new Set() };
  return { pressed: every.every((v) => chosen.has(v)), next: toggledAll(chosen, every) };
}
