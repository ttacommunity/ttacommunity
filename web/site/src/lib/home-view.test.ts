import { describe, expect, it } from 'vitest';
import { seatRankClass, toggled } from './home-view';

describe('seatRankClass', () => {
  it.each([
    ['the only seat', 50, [50], 'rank-gold'],
    ['a tie for everyone', 50, [50, 50, 50], 'rank-gold'],
    ['the winner of two', 90, [90, 40], 'rank-gold'],
    ['the loser of two', 40, [90, 40], 'rank-gray'],
    ['second of three', 70, [90, 70, 40], 'rank-silver'],
    ['last of three', 40, [90, 70, 40], 'rank-bronze'],
    ['second of four', 70, [90, 70, 60, 40], 'rank-silver'],
    ['third of four', 60, [90, 70, 60, 40], 'rank-bronze'],
    ['last of four', 40, [90, 70, 60, 40], 'rank-gray'],
  ] as const)('colours %s', (_, score, scores, want) => {
    expect(seatRankClass(score, scores)).toBe(want);
  });
});

describe('toggled', () => {
  it.each([
    ['adds an item not there', [1, 2], 3, [1, 2, 3]],
    ['removes an item there', [1, 2, 3], 2, [1, 3]],
    ['adds to an empty set', [], 7, [7]],
  ])('%s', (_, items, item, want) => {
    const before = new Set(items);
    expect([...toggled(before, item)]).toEqual(want);
    expect([...before]).toEqual(items);
  });
});
