import { describe, expect, it } from 'vitest';
import { divisionLevel, divisionTagClass } from './divisions';

describe('divisionLevel', () => {
  it.each([
    ['Grandmaster', 1],
    ['Master', 2],
    ['Diamond', 3],
    ['Platinum', 4],
    ['Gold', 5],
    ['Silver', 6],
    ['Bronze', 7],
    ['Emperor', 1],
    ['King 1', 2],
    ['King 2', 2],
    ['Prince 2', 3],
    ['Duke 1', 4],
    ['Marquess 1', 5],
    ['Count 1', 6],
    ['  grandmaster ', 1],
  ])('places %j at level %i', (name, level) => {
    expect(divisionLevel(name)).toBe(level);
  });

  it.each(['', 'Group A', 'Open', 'Kingdom'])('gives %j no level', (name) => {
    expect(divisionLevel(name)).toBeNull();
  });
});

describe('divisionTagClass', () => {
  it.each([
    ['Emperor', 'tag tag-level-1'],
    ['Silver', 'tag tag-level-6'],
    ['Group A', 'tag'],
  ])('draws %j as %j', (name, want) => {
    expect(divisionTagClass(name)).toBe(want);
  });
});
