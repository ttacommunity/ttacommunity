import { describe, expect, it } from 'vitest';
import { tierSpans } from './tier-bands';

describe('tierSpans', () => {
  const names = (lo: number, hi: number, at = (r: number) => r) => tierSpans(lo, hi, at).map((s) => [s.tier.code, s.start, s.end]);

  it.each([
    ['each tier the range meets, cut to the range', 1500, 1900, [['SLV', 1500, 1600], ['GLD', 1600, 1800], ['MST', 1800, 1900]]],
    ['one tier', 1650, 1700, [['GLD', 1650, 1700]]],
    ['the lowest tier unbounded below', -200, 100, [['BRZ', -200, 100]]],
    ['the highest tier unbounded above', 3000, 3500, [['SGM', 3000, 3500]]],
    ['an empty range', 1500, 1500, []],
  ])('%s', (_, lo, hi, want) => {
    expect(names(lo, hi)).toEqual(want);
  });

  it('places each edge where at puts its rating', () => {
    expect(names(1500, 1700, (r) => r / 100)).toEqual([['SLV', 15, 16], ['GLD', 16, 17]]);
  });
});
