import { describe, expect, it } from 'vitest';
import {
  RatingsApiError,
  currentRun,
  history,
  leaderboard,
  player,
  playerHistory,
  playerMatches,
  playerCareer,
  playerRivals,
  playerVersus,
} from './ratings';

interface Call {
  url: string;
  init?: RequestInit;
}

function fakeFetch(status: number, body: string): { fetch: typeof fetch; calls: Call[] } {
  const calls: Call[] = [];
  const fn = async (input: RequestInfo | URL, init?: RequestInit) => {
    calls.push({ url: String(input), init });
    return new Response(body, { status, headers: { 'Content-Type': 'application/json' } });
  };
  return { fetch: fn as typeof fetch, calls };
}

describe('currentRun', () => {
  it('reads the current run without credentials', async () => {
    const f = fakeFetch(200, '{"run_id":5,"published_at":"2026-09-27T04:52:18Z"}');
    const run = await currentRun('https://api.example', f.fetch);

    expect(run).toEqual({ run_id: 5, published_at: '2026-09-27T04:52:18Z' });
    expect(f.calls).toHaveLength(1);
    expect(f.calls[0]?.url).toBe('https://api.example/v1/runs/current');
    expect(f.calls[0]?.init?.credentials).toBe('omit');
  });

  it('accepts a base URL with a trailing slash', async () => {
    const f = fakeFetch(200, '{"run_id":5,"published_at":"2026-09-27T04:52:18Z"}');
    await currentRun('https://api.example/', f.fetch);
    expect(f.calls[0]?.url).toBe('https://api.example/v1/runs/current');
  });

  it("raises the API's message when nothing is published", async () => {
    const f = fakeFetch(503, '{"error":"no rating run has been published yet"}');
    const err = await currentRun('https://api.example', f.fetch).catch((e: unknown) => e);

    expect(err).toBeInstanceOf(RatingsApiError);
    expect(err).toMatchObject({ status: 503, message: 'no rating run has been published yet' });
  });

  it('raises a plain message when the error body is not JSON', async () => {
    const f = fakeFetch(502, '<html>Bad gateway</html>');
    const err = await currentRun('https://api.example', f.fetch).catch((e: unknown) => e);

    expect(err).toMatchObject({ status: 502, message: 'request failed (502)' });
  });
});

describe('leaderboard', () => {
  it('asks for one run, model and format without credentials', async () => {
    const body = '{"run_id":5,"model":"whr","format":2,"players":[]}';
    const f = fakeFetch(200, body);
    const board = await leaderboard('https://api.example', 5, 'whr', 2, f.fetch);

    expect(board).toEqual(JSON.parse(body));
    expect(f.calls[0]?.url).toBe('https://api.example/v1/runs/5/leaderboard?model=whr&format=2');
    expect(f.calls[0]?.init?.credentials).toBe('omit');
  });

  it("raises the API's message for a run that is not published", async () => {
    const f = fakeFetch(404, '{"error":"no published rating run has this id"}');
    const err = await leaderboard('https://api.example', 9, 'whr', 0, f.fetch).catch((e: unknown) => e);

    expect(err).toMatchObject({ status: 404, message: 'no published rating run has this id' });
  });
});

describe('history', () => {
  it('asks for a date, or for the previous period without one', async () => {
    const f = fakeFetch(200, '{"run_id":5,"model":"whr","format":2,"at":"2026-05-01","players":[]}');
    const got = await history('https://api.example', 5, 'whr', 2, '2026-05-01', f.fetch);
    await history('https://api.example', 5, 'glicko2_daneo', 0, null, f.fetch);

    expect(got.at).toBe('2026-05-01');
    expect(f.calls.map((c) => c.url)).toEqual([
      'https://api.example/v1/runs/5/history?model=whr&format=2&at=2026-05-01',
      'https://api.example/v1/runs/5/history?model=glicko2_daneo&format=0',
    ]);
    expect(f.calls[0]?.init?.credentials).toBe('omit');
  });
});

describe('player routes', () => {
  it('encode the name as one path segment and send no credentials', async () => {
    const f = fakeFetch(200, '{}');
    await player('https://api.example', 5, 'a/b c?', f.fetch);
    await playerHistory('https://api.example', 5, 'Ann', 3, f.fetch);
    await playerMatches('https://api.example', 5, 'Ann', 0, 'whr_softer', 2, f.fetch);
    await playerRivals('https://api.example', 5, 'Ann', 4, f.fetch);
    await playerCareer('https://api.example', 5, 'Ann', 2, f.fetch);
    await playerVersus('https://api.example', 5, 'Ann', 'b/c', f.fetch);

    expect(f.calls.map((c) => c.url)).toEqual([
      'https://api.example/v1/runs/5/players/a%2Fb%20c%3F',
      'https://api.example/v1/runs/5/players/Ann/history?format=3',
      'https://api.example/v1/runs/5/players/Ann/matches?format=0&model=whr_softer&page=2',
      'https://api.example/v1/runs/5/players/Ann/rivals?format=4',
      'https://api.example/v1/runs/5/players/Ann/career?format=2',
      'https://api.example/v1/runs/5/players/Ann/versus/b%2Fc',
    ]);
    expect(f.calls.every((c) => c.init?.credentials === 'omit')).toBe(true);
  });

  it("raise the API's message for an unknown player", async () => {
    const f = fakeFetch(404, '{"error":"no player has this name"}');
    const err = await player('https://api.example', 5, 'zed', f.fetch).catch((e: unknown) => e);
    expect(err).toMatchObject({ status: 404, message: 'no player has this name' });
  });
});
