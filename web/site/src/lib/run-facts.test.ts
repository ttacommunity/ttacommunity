import { describe, expect, it } from 'vitest';
import { count } from '@ttac/shared/numbers';
import type { Run } from '@ttac/shared/ratings';
import { runFacts } from './run-facts';

const run: Run = {
  run_id: 6,
  published_at: '2026-10-01T02:00:00Z',
  engine_version: 'a7677945',
  finished_at: '2026-10-01T01:10:00Z',
  games: 135393,
  players: 3526,
  first_game: '2017-09-25',
  last_game: '2026-08-31',
};

describe('runFacts', () => {
  it('gives the years, games, players and when the run was computed', () => {
    expect(runFacts(run, 'UTC')).toBe(` (2017–2026) • ${count(135393)} games • ${count(3526)} players • updated 1 Oct 2026, 01:10 (UTC)`);
  });

  it('gives the visits beside the players, with the days they cover', () => {
    const visits = { visits: 1190, days: 28, as_of: '2026-10-27T05:00:00Z' };
    expect(runFacts(run, 'UTC', visits)).toContain(` • ${count(3526)} players • ${count(1190)} visits (last 28 days) • `);
  });

  it('leaves out the visits when there is no count', () => {
    expect(runFacts(run, 'UTC', null)).not.toContain('visits');
  });

  it('times the update to the minute in the zone asked for, naming it', () => {
    expect(runFacts({ ...run, finished_at: '2026-09-30T23:30:00Z' }, 'Australia/Sydney')).toContain('updated 1 Oct 2026, 09:30 (Australia/Sydney)');
  });

  it('leaves out what the run did not record', () => {
    expect(runFacts({ ...run, games: null, players: null, first_game: null, finished_at: null }, 'UTC')).toBe('');
  });
});
