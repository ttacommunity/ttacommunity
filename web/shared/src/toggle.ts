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
