import { describe, expect, it } from 'vitest';
import { seasonSummary, stageRows } from './season-counts';

const stage = (label: string, note = '') => ({ label, original_name: label, note });
const count = (stage: string, number: number, games: number, finished: number, players = 2) => ({
  stage,
  number,
  games,
  finished,
  players,
});

// Stage 3 has games but none finished; Stage 4 none at all.
const STAGES = [stage('Stage 1'), stage('Stage 2', 'Best of three'), stage('Stage 10'), stage('Stage 3'), stage('Stage 4')];
const COUNT = {
  season: '2024',
  games: 9,
  players: 5,
  stages: [count('Stage 1', 1, 4, 4, 5), count('Stage 2', 2, 2, 2), count('Stage 10', 10, 1, 1), count('Stage 3', 3, 2, 0)],
};

describe('season counts', () => {
  it('lists only stages with finished games, the highest number first, with their finished games and players', () => {
    expect(stageRows(STAGES, COUNT)).toEqual([
      { ...stage('Stage 10'), games: 1, players: 2 },
      { ...stage('Stage 2', 'Best of three'), games: 2, players: 2 },
      { ...stage('Stage 1'), games: 4, players: 5 },
    ]);
  });

  it('lists nothing for a season with no games yet', () => {
    expect(stageRows(STAGES, undefined)).toEqual([]);
  });

  it.each([
    ['a season with stages', STAGES, COUNT, { stages: 3, games: 7, players: 5 }],
    ['a season with no games yet', [], undefined, { stages: 0, games: 0, players: 0 }],
  ] as const)('sums up %s: its stages with finished games, their finished games, its players', (_, stages, c, want) => {
    expect(seasonSummary([...stages], c)).toEqual(want);
  });
});
