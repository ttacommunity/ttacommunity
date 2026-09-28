/** A client for the public ratings API. Types come from the generated spec. */
import type { components } from '../api/schema';

export type Run = components['schemas']['Run'];
export type Model = components['schemas']['Model'];
export type Format = components['schemas']['Format'];
export type Leaderboard = components['schemas']['Leaderboard'];
export type LeaderboardRow = components['schemas']['LeaderboardRow'];
export type History = components['schemas']['History'];
export type TitleCode = NonNullable<components['schemas']['Title']>['code'];

/** An error response from the API, carrying its status and message. */
export class RatingsApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
  ) {
    super(message);
    this.name = 'RatingsApiError';
  }
}

/** Returns the run the site shows now. */
export async function currentRun(base: string, fetchFn: typeof fetch = fetch): Promise<Run> {
  return get<Run>(fetchFn, base, '/v1/runs/current');
}

/** Returns every rated player in one published run, model and format, in rank order. */
export async function leaderboard(
  base: string,
  run: number,
  model: Model,
  format: Format,
  fetchFn: typeof fetch = fetch,
): Promise<Leaderboard> {
  const query = new URLSearchParams({ model, format: String(format) });
  return get<Leaderboard>(fetchFn, base, `/v1/runs/${run}/leaderboard?${query}`);
}

/** Returns each player's rating at a past date, or at the run's previous period when date is null. */
export async function history(
  base: string,
  run: number,
  model: Model,
  format: Format,
  date: string | null,
  fetchFn: typeof fetch = fetch,
): Promise<History> {
  const query = new URLSearchParams({ model, format: String(format) });
  if (date !== null) query.set('at', date);
  return get<History>(fetchFn, base, `/v1/runs/${run}/history?${query}`);
}

// Public routes take no credentials, so none are ever sent.
async function get<T>(fetchFn: typeof fetch, base: string, path: string): Promise<T> {
  const res = await fetchFn(base.replace(/\/+$/, '') + path, { credentials: 'omit' });
  if (!res.ok) {
    const body: unknown = await res.json().catch(() => null);
    const message =
      body !== null && typeof body === 'object' && 'error' in body && typeof body.error === 'string'
        ? body.error
        : `request failed (${res.status})`;
    throw new RatingsApiError(res.status, message);
  }
  return (await res.json()) as T;
}
