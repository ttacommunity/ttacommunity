import { describe, expect, it } from 'vitest';
import { allButton, toggled, toggledAll, toggledSet } from './toggle';

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

describe('toggledAll', () => {
  it.each([
    ['selects every value when some are chosen', ['a'], ['a', 'b', 'c']],
    ['selects every value when none are chosen', [], ['a', 'b', 'c']],
    ['clears the choice when every value is chosen', ['c', 'a', 'b'], []],
  ])('%s', (_, chosen, want) => {
    expect([...toggledAll(new Set(chosen), ['a', 'b', 'c'])]).toEqual(want);
  });
});

describe('allButton', () => {
  it.each<[string, string[], boolean, { pressed: boolean; next: string[] }]>([
    ['is pressed when every value is chosen', ['a', 'b', 'c'], false, { pressed: true, next: [] }],
    ['chooses every value when some are chosen', ['a'], false, { pressed: false, next: ['a', 'b', 'c'] }],
    ['is pressed when none is chosen and none means all', [], true, { pressed: true, next: [] }],
    ['clears the choice when none means all', ['a'], true, { pressed: false, next: [] }],
  ])('%s', (_, chosen, noneMeansAll, want) => {
    const got = allButton(new Set(chosen), ['a', 'b', 'c'], noneMeansAll);
    expect({ pressed: got.pressed, next: [...got.next] }).toEqual(want);
  });
});
