import { describe, expect, it } from 'vitest';
import { ladderBoard, podiumBoard } from './trophyboard';
import { boardCells } from './tournament-series';

const season = (...podium: [number, string][]) => ({ podium: podium.map(([place, name]) => ({ place, name })) });

describe('podiumBoard', () => {
  it("counts each player's podiums over the seasons, most golds first, then silvers, bronzes and name", () => {
    const board = podiumBoard([
      season([1, 'ann'], [2, 'bob'], [3, 'cat']),
      season([1, 'bob'], [2, 'ann'], [3, 'dan']),
      season([1, 'ann'], [3, 'bob']),
    ]);
    expect(board.map((r) => [r.name, r.gold, r.silver, r.bronze])).toEqual([
      ['ann', 2, 1, 0],
      ['bob', 1, 1, 1],
      ['cat', 0, 0, 1],
      ['dan', 0, 0, 1],
    ]);
    expect(boardCells(board).headings).toEqual(['🥇 Gold', '🥈 Silver', '🥉 Bronze', 'Total']);
  });

  it('is empty for a tournament with no podiums yet', () => {
    expect(podiumBoard([season(), season()])).toEqual([]);
  });
});

describe('ladderBoard', () => {
  it("shows a ladder's top-tier wins, most first, under their own heading", () => {
    const board = ladderBoard([
      { name: 'Grozz', wins: 3.5, source: 'Hall of Fame' },
      { name: 'wolvs', wins: 10, source: 'Hall of Fame' },
    ]);
    expect(board.map((r) => [r.name, r.wins])).toEqual([
      ['wolvs', 10],
      ['Grozz', 3.5],
    ]);
    expect(boardCells(board)).toMatchObject({ headings: ['🏆 Top-tier wins'], rows: [['10'], ['3.5']] });
  });
});
