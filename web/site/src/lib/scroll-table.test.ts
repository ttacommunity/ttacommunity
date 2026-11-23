import { describe, expect, it } from 'vitest';
import { scrollEdges } from './scroll-table';

describe('scrollEdges', () => {
  it.each([
    ['a table that fits', 0, 400, 400, { start: false, end: false }],
    ['a table a pixel wider, from rounding', 0, 400, 401, { start: false, end: false }],
    ['a wider table at its start', 0, 400, 900, { start: false, end: true }],
    ['a wider table part way along', 200, 400, 900, { start: true, end: true }],
    ['a wider table at its end', 500, 400, 900, { start: true, end: false }],
    ['a wider table at its end, scrolled to a fraction', 499.4, 400, 900, { start: true, end: false }],
  ])('%s', (_, left, width, full, want) => {
    expect(scrollEdges(left, width, full)).toEqual(want);
  });
});
