import { describe, expect, it } from 'vitest';
import { divisionLevel, divisionTagClass } from './divisions';

describe('divisionLevel', () => {
  it.each([
    ['Grandmaster', 'International Championship', 1],
    ['Master 2', 'International Championship', 2],
    ['Diamond', 'Intermezzo Championship', 3],
    ['Platinum 4', 'international', 4],
    ['Gold 8', 'Intermezzo Championship', 5],
    ['Silver 16', 'International Championship', 6],
    ['Bronze 32', 'International Championship', 7],
    ['Wood 15', 'International Championship', 8],
    ['Gold 3', 'Survivors Cup', 5],
    ['Emperor', 'Royal League', 1],
    ['King', 'Royal League', 2],
    ['Prince 2', 'royal_league', 3],
    ['Duke 3', 'Royal League', 4],
    ['Marquess 5', 'Royal League', 5],
    ['Count 8', 'Royal League', 6],
    ['Viscount 13', 'Royal League', 7],
    ['Baron 22', 'Royal League', 8],
    ['  grandmaster ', 'International Championship', 1],
  ])('places %j in %j at level %i', (name, tournament, level) => {
    expect(divisionLevel(name, tournament)).toBe(level);
  });

  it.each([
    ['Emperor', 'International Championship'],
    ['Grandmaster', 'Royal League'],
    ['Pool', 'World Championship'],
    ['Group A', 'Survivors Cup'],
    ['', 'Royal League'],
  ])('gives %j in %j no level', (name, tournament) => {
    expect(divisionLevel(name, tournament)).toBeNull();
  });
});

describe('divisionTagClass', () => {
  it.each([
    ['Emperor', 'Royal League', 'tag tag-level-1'],
    ['Silver 3', 'International Championship', 'tag tag-level-6'],
    ['Pool', 'World Championship', 'tag'],
  ])('draws %j in %j as %j', (name, tournament, want) => {
    expect(divisionTagClass(name, tournament)).toBe(want);
  });
});
