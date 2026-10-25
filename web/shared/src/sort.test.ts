import { describe, expect, it } from 'vitest';
import { nextSort, sortRows, type SortColumns } from './sort';

type Row = { name: string; points: number; rating: number | null; round: number; score: number };
const rows: Row[] = [
  { name: 'cy', points: 1, rating: 1600, round: 2, score: 90 },
  { name: 'Ann', points: 2, rating: null, round: 1, score: 200 },
  { name: 'bo', points: 1, rating: 1700, round: 1, score: 250 },
];
const columns: SortColumns<Row, 'name' | 'points' | 'rating' | 'game'> = {
  name: { value: (r) => r.name, first: 'asc' },
  points: { value: (r) => r.points, first: 'desc' },
  rating: { value: (r) => r.rating, first: 'desc' },
  game: { value: (r) => [r.round, -r.score], first: 'asc' },
};

describe('nextSort', () => {
  it.each([
    ['a column first sorts its first way', null, 'points', { col: 'points', desc: true }],
    ['the same column again sorts the other way', { col: 'points', desc: true }, 'points', { col: 'points', desc: false }],
    ['a third time returns the table to its own order', { col: 'points', desc: false }, 'points', null],
    ['another column starts at its own first way', { col: 'points', desc: true }, 'name', { col: 'name', desc: false }],
  ] as const)('%s', (_name, current, col, want) => {
    expect(nextSort(current, col, columns)).toEqual(want);
  });
});

describe('sortRows', () => {
  it.each([
    ['the table own order when not sorted', null, ['cy', 'Ann', 'bo']],
    ['names A to Z, whatever their case', { col: 'name', desc: false }, ['Ann', 'bo', 'cy']],
    ['numbers highest first, ties in the table own order', { col: 'points', desc: true }, ['Ann', 'cy', 'bo']],
    ['numbers lowest first the other way', { col: 'points', desc: false }, ['cy', 'bo', 'Ann']],
    ['a missing value last either way', { col: 'rating', desc: false }, ['cy', 'bo', 'Ann']],
    ['several values in turn', { col: 'game', desc: false }, ['bo', 'Ann', 'cy']],
  ] as const)('keeps %s', (_name, sort, want) => {
    expect(sortRows(rows, columns, sort).map((r) => r.name)).toEqual(want);
  });
});
