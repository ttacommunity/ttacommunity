import { describe, expect, it } from 'vitest';
import { seasonSummary, stageRows } from './season-counts';

const stage = (label: string, number: number, games: number, finished: number, players = 2, note = '') => ({
  label,
  original_name: label,
  note,
  number,
  games,
  finished,
  players,
});

// Stage 3 has games but none finished; Stage 4 none at all.
const STAGES = [stage('Stage 1', 1, 4, 4, 5), stage('Stage 2', 2, 2, 2, 2, 'Best of three'), stage('Stage 10', 10, 1, 1), stage('Stage 3', 3, 2, 0), stage('Stage 4', 4, 0, 0, 0)];

describe('season counts', () => {
  it('lists only stages with finished games, the highest number first', () => {
    expect(stageRows(STAGES).map((s) => s.label)).toEqual(['Stage 10', 'Stage 2', 'Stage 1']);
  });

  it('orders stages of one number by name, the highest first', () => {
    expect(stageRows([stage('Final A', 0, 1, 1), stage('Final B', 0, 1, 1)]).map((s) => s.label)).toEqual(['Final B', 'Final A']);
  });

  it.each([
    ['a season with stages', { players: 5, stages: STAGES }, { stages: 3, games: 7, players: 5 }],
    ['a season with no games yet', { players: 0, stages: [] }, { stages: 0, games: 0, players: 0 }],
  ])('sums up %s: its stages with finished games, their finished games, its players', (_, season, want) => {
    expect(seasonSummary(season)).toEqual(want);
  });
});
