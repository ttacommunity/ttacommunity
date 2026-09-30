import { describe, expect, it } from 'vitest';
import type { CommunityRow } from '@ttac/shared/ratings';
import { countingColumns, pageWindow, parseCommunityView, titleMark, tournamentSums } from './community';
import { EDITIONS } from './community-editions';

const row = (r: Partial<CommunityRow>): CommunityRow => ({
  rank: 1, name: 'ann', country: null, title: null, points: 0, events: 0, scores: {}, ...r,
});

describe('parseCommunityView', () => {
  it("reads upstream's edition names and falls back to 2026", () => {
    expect(parseCommunityView(new URLSearchParams('edition=y2024')).edition).toBe('2024');
    expect(parseCommunityView(new URLSearchParams('edition=race')).edition).toBe('race26');
    expect(parseCommunityView(new URLSearchParams('edition=1999')).edition).toBe('2026');
  });

  it('keeps 10 to 200 rows a page, all as 2000, and the standard view unless expanded', () => {
    expect(parseCommunityView(new URLSearchParams('per_page=5')).perPage).toBe(10);
    expect(parseCommunityView(new URLSearchParams('per_page=all&view=EXPANDED')).perPage).toBe(2000);
    expect(parseCommunityView(new URLSearchParams('per_page=all&view=EXPANDED')).view).toBe('expanded');
    expect(parseCommunityView(new URLSearchParams('page=x&view=grid'))).toMatchObject({ page: 1, perPage: 50, view: 'standard' });
  });
});

describe('countingColumns', () => {
  it("keeps a player's 8 best, ties in the sheet's column order", () => {
    const scores = { a: 10, b: 50, c: 50, d: 1, e: 2, f: 3, g: 4, h: 5, i: 6, j: 0 };
    expect(countingColumns(scores, ['a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j'])).toEqual(['b', 'c', 'a', 'i', 'h', 'g', 'f', 'e']);
    expect(countingColumns({ x: 500, y: 500 }, ['y', 'x'])).toEqual(['y', 'x']);
  });
});

describe('tournamentSums', () => {
  it("sums each tournament's positive events and says whether one counts", () => {
    const summary = EDITIONS['2024'].summary.filter((t) => t.id === 'ic' || t.id === 'wrld' || t.id === 'qd');
    const got = tournamentSums({ ic_24: 1000, ic_25: 510, wrld: 0, qd: 20 }, summary, ['ic_25']);
    expect(got).toEqual({ wrld: null, ic: { sum: 1510, counting: true }, qd: { sum: 20, counting: false } });
  });
});

describe('titleMark', () => {
  it("shows WC for the edition's champion, GM, M, P and G, and nothing else, as upstream", () => {
    expect(titleMark(row({ name: 'Martin_Pecheur', title: 'GM' }), EDITIONS['2024'])).toBe('WC');
    expect(titleMark(row({ title: 'M' }), EDITIONS['2024'])).toBe('M');
    expect(titleMark(row({ title: 'SGM' }), EDITIONS['2024'])).toBeNull();
    expect(titleMark(row({ title: 'S' }), EDITIONS['2024'])).toBeNull();
  });
});

describe('pageWindow', () => {
  it('lists three pages either side, as upstream', () => {
    expect(pageWindow(1, 30)).toEqual([1, 2, 3, 4]);
    expect(pageWindow(10, 30)).toEqual([7, 8, 9, 10, 11, 12, 13]);
    expect(pageWindow(30, 30)).toEqual([27, 28, 29, 30]);
  });
});
