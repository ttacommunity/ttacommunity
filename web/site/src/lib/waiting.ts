/** What a page says while its data loads, or when it cannot: the same words everywhere. */

/** A load's state as the pages keep it; message is set on an error. */
export type LoadState = { kind: string; message?: string };

/** waitingText returns "Loading {what}…", "Could not load {what}: {why}", or null once loaded. */
export function waitingText(load: LoadState, what: string): string | null {
  if (load.kind === 'loading') return `Loading ${what}…`;
  if (load.kind === 'error') return `Could not load ${what}: ${load.message ?? 'unknown error'}`;
  return null;
}
