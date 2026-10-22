import { describe, expect, it } from 'vitest';
import {
  catalogueGames,
  RatingsApiError,
  activity,
  tournamentSeries,
  hallOfFame,
  catalogue,
  catalogueTournament,
  recentPodiums,
  agreement,
  seatAnalysis,
  calibration,
  comparedPlayers,
  currentRun,
  distribution,
  history,
  leaderboard,
  player,
  playerHistory,
  playerMatches,
  communityLeaderboard,
  errorFields,
  titleName,
  playerAchievements,
  playerCareer,
  playerMatrix,
  playerSeats,
  playerRivals,
  playerVersus,
  yearLeaderboard,
  years,
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

  it("quotes a server error's request id in its message, to report it by", async () => {
    const f = fakeFetch(500, '{"error":"internal error","request_id":"0123456789abcdef"}');
    const err = await currentRun('https://api.example', f.fetch).catch((e: unknown) => e);

    expect(err).toMatchObject({ status: 500, message: expect.stringContaining('0123456789abcdef') });
  });

  it('asks once for every caller on the page', async () => {
    const f = fakeFetch(200, '{"run_id":5,"published_at":"2026-09-27T04:52:18Z"}');
    const [a, b] = await Promise.all([currentRun('https://api.example', f.fetch), currentRun('https://api.example', f.fetch)]);
    await currentRun('https://api.example', f.fetch);

    expect(a).toEqual(b);
    expect(f.calls).toHaveLength(1);
  });

  it('asks again after a failure', async () => {
    let status = 500;
    const calls: string[] = [];
    const fetchFn = (async (input: RequestInfo | URL) => {
      calls.push(String(input));
      return new Response(status === 200 ? '{"run_id":5,"published_at":"2026-09-27T04:52:18Z"}' : '{"error":"down"}', { status });
    }) as typeof fetch;

    await expect(currentRun('https://api.example', fetchFn)).rejects.toBeInstanceOf(RatingsApiError);
    status = 200;
    await expect(currentRun('https://api.example', fetchFn)).resolves.toMatchObject({ run_id: 5 });
    expect(calls).toHaveLength(2);
  });

  it('raises a plain message when the error body is not JSON', async () => {
    const f = fakeFetch(502, '<html>Bad gateway</html>');
    const err = await currentRun('https://api.example', f.fetch).catch((e: unknown) => e);

    expect(err).toMatchObject({ status: 502, message: 'request failed (502)' });
  });
});

describe('leaderboard', () => {
  it('asks for one run, model and format without credentials', async () => {
    const body = '{"run_id":5,"model":"glicko2_std","format":2,"players":[]}';
    const f = fakeFetch(200, body);
    const board = await leaderboard('https://api.example', 5, 'glicko2_std', 2, f.fetch);

    expect(board).toEqual(JSON.parse(body));
    expect(f.calls[0]?.url).toBe('https://api.example/v1/runs/5/leaderboard?model=glicko2_std&format=2');
    expect(f.calls[0]?.init?.credentials).toBe('omit');
  });

  it("raises the API's message for a run that is not published", async () => {
    const f = fakeFetch(404, '{"error":"no published rating run has this id"}');
    const err = await leaderboard('https://api.example', 9, 'glicko2_std', 0, f.fetch).catch((e: unknown) => e);

    expect(err).toMatchObject({ status: 404, message: 'no published rating run has this id' });
  });
});

describe('history', () => {
  it('asks for a date, or for the previous period without one', async () => {
    const f = fakeFetch(200, '{"run_id":5,"model":"glicko2_std","format":2,"at":"2026-05-01","players":[]}');
    const got = await history('https://api.example', 5, 'glicko2_std', 2, '2026-05-01', f.fetch);
    await history('https://api.example', 5, 'glicko2_daneo', 0, null, f.fetch);

    expect(got.at).toBe('2026-05-01');
    expect(f.calls.map((c) => c.url)).toEqual([
      'https://api.example/v1/runs/5/history?model=glicko2_std&format=2&at=2026-05-01',
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
    await playerMatches('https://api.example', 5, 'Ann', 0, 'glicko2_daneo_softer', 2, f.fetch);
    await playerRivals('https://api.example', 5, 'Ann', 4, f.fetch);
    await playerCareer('https://api.example', 5, 'Ann', 2, 'glicko2_daneo_softer', f.fetch);
    await playerVersus('https://api.example', 5, 'Ann', 'b/c', f.fetch);
    await playerMatrix('https://api.example', 5, 'Ann', 3, null, f.fetch);
    await playerMatrix('https://api.example', 5, 'Ann', 0, 2024, f.fetch);
    await playerSeats('https://api.example', 5, 'Ann', false, f.fetch);
    await playerSeats('https://api.example', 5, 'Ann', true, f.fetch);

    expect(f.calls.map((c) => c.url)).toEqual([
      'https://api.example/v1/runs/5/players/a%2Fb%20c%3F',
      'https://api.example/v1/runs/5/players/Ann/history?format=3',
      'https://api.example/v1/runs/5/players/Ann/matches?format=0&model=glicko2_daneo_softer&page=2',
      'https://api.example/v1/runs/5/players/Ann/rivals?format=4',
      'https://api.example/v1/runs/5/players/Ann/career?format=2&model=glicko2_daneo_softer',
      'https://api.example/v1/runs/5/players/Ann/versus/b%2Fc',
      'https://api.example/v1/runs/5/players/Ann/matrix?format=3',
      'https://api.example/v1/runs/5/players/Ann/matrix?format=0&year=2024',
      'https://api.example/v1/runs/5/players/Ann/seats',
      'https://api.example/v1/runs/5/players/Ann/seats?unrated=true',
    ]);
    expect(f.calls.every((c) => c.init?.credentials === 'omit')).toBe(true);
  });

  it("raise the API's message for an unknown player", async () => {
    const f = fakeFetch(404, '{"error":"no player has this name"}');
    const err = await player('https://api.example', 5, 'zed', f.fetch).catch((e: unknown) => e);
    expect(err).toMatchObject({ status: 404, message: 'no player has this name' });
  });
});

describe('year routes', () => {
  it('ask for the years and one year\'s board without credentials', async () => {
    const f = fakeFetch(200, '{}');
    await years('https://api.example', 5, f.fetch);
    await yearLeaderboard('https://api.example', 5, 2024, 'glicko2_daneo_softer', 3, f.fetch);
    expect(f.calls.map((c) => c.url)).toEqual([
      'https://api.example/v1/runs/5/years',
      'https://api.example/v1/runs/5/years/2024/leaderboard?model=glicko2_daneo_softer&format=3',
    ]);
    expect(f.calls.every((c) => c.init?.credentials === 'omit')).toBe(true);
  });
});

describe('analysis routes', () => {
  it('ask for the calibration, distribution and agreement without credentials', async () => {
    const f = fakeFetch(200, '{}');
    await calibration('https://api.example', 5, f.fetch);
    await distribution('https://api.example', 5, 3, f.fetch);
    await agreement('https://api.example', 5, 0, f.fetch);
    expect(f.calls.map((c) => c.url)).toEqual([
      'https://api.example/v1/runs/5/analysis/calibration',
      'https://api.example/v1/runs/5/analysis/distribution?format=3',
      'https://api.example/v1/runs/5/analysis/agreement?format=0',
    ]);
    expect(f.calls.every((c) => c.init?.credentials === 'omit')).toBe(true);
  });
});

describe('analysis pages', () => {
  it('ask for compared players and the activity without credentials', async () => {
    const f = fakeFetch(200, '{}');
    await comparedPlayers('https://api.example', 5, 'alder440, IT', 3, f.fetch);
    await comparedPlayers('https://api.example', 5, '', 0, f.fetch);
    await activity('https://api.example', 5, f.fetch);
    await seatAnalysis('https://api.example', 5, 'grandmaster', 2, f.fetch);
    expect(f.calls.map((c) => c.url)).toEqual([
      'https://api.example/v1/runs/5/analysis/players?format=3&search=alder440%2C+IT',
      'https://api.example/v1/runs/5/analysis/players?format=0',
      'https://api.example/v1/runs/5/analysis/activity',
      'https://api.example/v1/runs/5/analysis/seats?tier=grandmaster&format=2',
    ]);
    expect(f.calls.every((c) => c.init?.credentials === 'omit')).toBe(true);
  });
});

describe('tournament pages', () => {
  it('ask for a series without credentials', async () => {
    const f = fakeFetch(200, '{}');
    await tournamentSeries('https://api.example', 5, 'international', f.fetch);
    await hallOfFame('https://api.example', 5, f.fetch);
    await playerAchievements('https://api.example', 5, 'a/b c', f.fetch);
    await communityLeaderboard('https://api.example', 5, 'race26', f.fetch);
    expect(f.calls.map((c) => c.url)).toEqual([
      'https://api.example/v1/runs/5/tournaments/international',
      'https://api.example/v1/runs/5/tournaments',
      'https://api.example/v1/runs/5/players/a%2Fb%20c/achievements',
      'https://api.example/v1/runs/5/community/race26',
    ]);
    expect(f.calls.every((c) => c.init?.credentials === 'omit')).toBe(true);
  });

  it('ask for the catalogue and one tournament without credentials', async () => {
    const f = fakeFetch(200, '{}');
    await catalogue('https://api.example', 5, f.fetch);
    await catalogueTournament('https://api.example', 5, 'slow_burn', f.fetch);
    await catalogueTournament('https://api.example', 5, 'a/b', f.fetch);
    expect(f.calls.map((c) => c.url)).toEqual([
      'https://api.example/v1/runs/5/catalogue',
      'https://api.example/v1/runs/5/catalogue/slow_burn',
      'https://api.example/v1/runs/5/catalogue/a%2Fb',
    ]);
    expect(f.calls.every((c) => c.init?.credentials === 'omit')).toBe(true);
  });

  it('ask for the recent podiums without credentials', async () => {
    const f = fakeFetch(200, '{}');
    await recentPodiums('https://api.example', 5, f.fetch);
    expect(f.calls.map((c) => c.url)).toEqual(['https://api.example/v1/runs/5/podiums']);
    expect(f.calls[0]?.init?.credentials).toBe('omit');
  });

  it('ask for a page of games with only the filters chosen', async () => {
    const f = fakeFetch(200, '{}');
    await catalogueGames('https://api.example', 5, 'worlds', { page: 1 }, f.fetch);
    await catalogueGames('https://api.example', 5, 'worlds', { season: '2022 (Age A)', stage: 'Stage 1', group: 'Group 3', player: 'Ember_Wolf', coded: true, page: 2 }, f.fetch);
    expect(f.calls.map((c) => c.url)).toEqual([
      'https://api.example/v1/runs/5/catalogue/worlds/games?page=1',
      'https://api.example/v1/runs/5/catalogue/worlds/games?page=2&season=2022+%28Age+A%29&stage=Stage+1&group=Group+3&player=Ember_Wolf&coded=true',
    ]);
    expect(f.calls.every((c) => c.init?.credentials === 'omit')).toBe(true);
  });
});

describe('errorFields', () => {
  it.each([
    ['an error with its request id', { error: 'internal error', request_id: '0123456789abcdef' }, 'internal error', '0123456789abcdef'],
    ['an error alone', { error: 'unknown model' }, 'unknown model', undefined],
    ['not an object', 'Bad Gateway', undefined, undefined],
    ['no body', null, undefined, undefined],
    ['fields that are not strings', { error: 404, request_id: ['x'] }, undefined, undefined],
  ])('%s', (_, body, error, requestId) => {
    expect(errorFields(body)).toEqual({ error, requestId });
  });
});

describe('titleName', () => {
  it.each([
    ['WC', 'World Champion'],
    ['GM', 'Grandmaster'],
    ['M', 'Master'],
    ['P', 'Platinum'],
    ['G', 'Gold'],
    ['S', 'Silver'],
    ['B', 'Bronze'],
    ['W', 'Wood'],
  ] as const)('names %s as upstream does', (code, name) => {
    expect(titleName(code)).toBe(name);
  });
});
