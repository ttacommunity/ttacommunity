import { describe, expect, it } from 'vitest';
import { seatPlace } from './home-view';

describe('seatPlace', () => {
  it.each([
    ['the only seat', 50, [50], 1],
    ['a tie for everyone', 50, [50, 50, 50], 1],
    ['the winner of two', 90, [90, 40], 1],
    ['the loser of two', 40, [90, 40], 'last'],
    ['second of three', 70, [90, 70, 40], 2],
    ['last of three', 40, [90, 70, 40], 3],
    ['second of four', 70, [90, 70, 60, 40], 2],
    ['third of four', 60, [90, 70, 60, 40], 3],
    ['last of four', 40, [90, 70, 60, 40], 'last'],
  ] as const)('places %s', (_, score, scores, want) => {
    expect(seatPlace(score, scores)).toBe(want);
  });
});
