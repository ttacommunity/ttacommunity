import { describe, expect, it } from 'vitest';
import { toggled, toggledSet } from './toggle';

describe('toggled', () => {
  it.each([
    ['adds one not there, at the end', ['a'], 'b', ['a', 'b']],
    ['removes one there', ['a', 'b'], 'a', ['b']],
  ])('%s', (_, list, item, want) => {
    expect(toggled(list, item)).toEqual(want);
  });
});

describe('toggledSet', () => {
  it.each([
    ['adds one not there', [1], 2, [1, 2]],
    ['removes one there', [1, 2], 1, [2]],
    ['adds to an empty set', [], 7, [7]],
  ])('%s', (_, items, item, want) => {
    expect([...toggledSet(new Set(items), item)]).toEqual(want);
  });

  it('leaves the set it was given as it was', () => {
    const items = new Set([1]);
    toggledSet(items, 1);
    expect([...items]).toEqual([1]);
  });
});
