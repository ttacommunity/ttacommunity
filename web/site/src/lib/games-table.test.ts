import { describe, expect, it } from 'vitest';
import { gamesTag, pageOf } from './games-table';

describe('pageOf', () => {
  const rows = Array.from({ length: 120 }, (_, i) => i);
  it.each([
    ['the first page', 1, 0, 50, 3],
    ['the last, short page', 3, 100, 20, 3],
    ['a page past the end, as the last', 9, 100, 20, 3],
    ['a page before the start, as the first', 0, 0, 50, 3],
  ])('gives %s', (_name, page, first, length, pages) => {
    const got = pageOf(rows, page);
    expect([got.rows[0], got.rows.length, got.pages]).toEqual([first, length, pages]);
  });
  it('gives one empty page for no rows', () => {
    expect(pageOf([], 1)).toEqual({ rows: [], page: 1, pages: 1 });
  });
});

describe('gamesTag', () => {
  it.each([
    ['every game', 1234, 56, false, '1,234 games · 56 with a replay code'],
    ['only coded games', 56, 56, true, '56 games with a replay code'],
  ])('counts %s', (_name, total, coded, onlyCoded, want) => {
    expect(gamesTag(total, coded, onlyCoded)).toBe(want);
  });
});
