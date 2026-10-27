// What a page is loading: the state of a request, and the message a failed one shows.
import { RatingsApiError } from './ratings';

/** Load is something a page is loading, failed to load and why, or has. */
export type Load<T> = { kind: 'loading' } | { kind: 'error'; message: string } | { kind: 'ready'; data: T };

/** Found is a Load of something the address names, which may not be there. */
export type Found<T> = Load<T> | { kind: 'missing' };

/** errorText returns what a failure says: an Error's message, or anything else as text. */
export function errorText(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}

/** failed turns a failed request into a Found: missing when the API answered 404, else an error. */
export function failed(e: unknown): Found<never> {
  if (e instanceof RatingsApiError && e.status === 404) return { kind: 'missing' };
  return { kind: 'error', message: errorText(e) };
}

/** settled waits for a request and gives its answer as ready, or its failure as an error; it never rejects. */
export async function settled<T>(request: Promise<T>): Promise<Load<T>> {
  try {
    return { kind: 'ready', data: await request };
  } catch (e: unknown) {
    return { kind: 'error', message: errorText(e) };
  }
}
