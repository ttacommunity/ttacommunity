import { describe, expect, it } from 'vitest';
import { DEFAULT_KINDS, checkSummary, differenceRows, placesText, type StandingsCheck } from './standings-check';

const none = { p2: null, p3: null, p4: null };
const diff = (player: string, kind: StandingsCheck['stages'][number]['differences'][number]['kind'], extra = {}) => ({
  cge_user: 1,
  player,
  kind,
  cge_games: 6,
  held_games: 6,
  held_never_played: 0,
  cge_points: 10,
  cge_places: none,
  held_places: none,
  ...extra,
});

const check: StandingsCheck = {
  stages_only_on_cge: 18,
  stages: [
    {
      cge_tournament: 19,
      cge_season: 4,
      name: 'Intermezzo Championship',
      players: 10,
      agree: 7,
      differences: [diff('zed', 'places'), diff('Amy', 'places_after_a_timeout'), diff('bob', 'games', { held_games: 5 })],
    },
    { cge_tournament: 4127, cge_season: 6, name: '', players: 4, agree: 3, differences: [diff('cy', 'not_held')] },
    { cge_tournament: 21, cge_season: 7, name: 'International Championship', players: 5, agree: 5, differences: [] },
  ],
};

describe('checkSummary', () => {
  it('adds up the players who agree over every stage', () => {
    expect(checkSummary(check)).toMatchObject({ stages: 3, onlyOnCGE: 18, players: 19, agree: 15, share: '78.9%' });
  });

  it('counts each kind of difference, every kind listed even when none', () => {
    const kinds = Object.fromEntries(checkSummary(check).byKind.map((k) => [k.kind, k.count]));
    expect(kinds).toEqual({ games: 1, places: 1, places_after_a_timeout: 1, not_held: 1, not_on_cge: 0 });
  });

  it('gives no share when nothing was checked', () => {
    expect(checkSummary({ stages: [], stages_only_on_cge: 0 }).share).toBe('');
  });
});

describe('differenceRows', () => {
  it('hides the differences CGE makes by letting its AI place a timed-out seat, by default', () => {
    const players = differenceRows(check, DEFAULT_KINDS).map((r) => r.player);
    expect(players).not.toContain('Amy');
    expect(players).toHaveLength(3);
  });

  it('lists the kinds asked for, by stage in CGE order, players alphabetically within one', () => {
    expect(differenceRows(check, new Set(['games', 'places', 'places_after_a_timeout', 'not_held'])).map((r) => r.player)).toEqual([
      'Amy',
      'bob',
      'zed',
      'cy',
    ]);
  });

  it("names a stage by CGE's tournament, or its number when CGE's catalogue lacks it", () => {
    const stages = differenceRows(check, DEFAULT_KINDS).map((r) => r.stage);
    expect(stages).toContain('Intermezzo Championship (19), stage 4');
    expect(stages).toContain('CGE tournament 4127, stage 6');
  });

  it('says how many held games no one played', () => {
    const row = differenceRows(
      { stages_only_on_cge: 0, stages: [{ ...check.stages[0], differences: [diff('ann', 'games', { held_games: 6, held_never_played: 2 })] }] },
      DEFAULT_KINDS,
    )[0];
    expect(row.held).toContain('6 games (2 never played)');
  });
});

describe('placesText', () => {
  it('lists each size of game played, firsts first', () => {
    expect(placesText({ p2: [1, 0], p3: null, p4: [0, 0, 1, 2] })).toBe('2p: 1 0 · 4p: 0 0 1 2');
  });

  it('says so when no game was placed', () => {
    expect(placesText(none)).toBe('no places');
  });
});
