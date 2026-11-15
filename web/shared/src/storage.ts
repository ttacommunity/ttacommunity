/** The browser's local storage, where the browser lets a page use it. */

/** browserStorage is the browser's local storage, or null where touching it throws (blocked site data). */
export function browserStorage(): Storage | null {
  try {
    return window.localStorage;
  } catch {
    return null;
  }
}
