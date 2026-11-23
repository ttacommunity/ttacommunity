import { describe, expect, it } from 'vitest';
import { ROWS_SHOWN, cutsRows, shownRows } from './show-all';

const rows = (n: number) => Array.from({ length: n }, (_, i) => i + 1);

describe('shownRows', () => {
  it.each([
    ['no rows', 0, false, 0],
    ['fewer than the cut', 5, false, 5],
    ['exactly the cut', ROWS_SHOWN, false, ROWS_SHOWN],
    ['one more than the cut', ROWS_SHOWN + 1, false, ROWS_SHOWN],
    ['many, cut', 64, false, ROWS_SHOWN],
    ['many, expanded', 64, true, 64],
  ])('%s: %i rows, expanded %s, shows %i', (_, n, expanded, want) => {
    expect(shownRows(rows(n), expanded)).toHaveLength(want);
  });

  it('keeps the first rows in order', () => {
    expect(shownRows(rows(12), false)).toEqual(rows(ROWS_SHOWN));
  });
});

describe('cutsRows', () => {
  it.each([
    [0, false],
    [ROWS_SHOWN, false],
    [ROWS_SHOWN + 1, true],
  ])('%i rows: %s', (n, want) => {
    expect(cutsRows(n)).toBe(want);
  });
});
