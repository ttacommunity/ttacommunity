import { describe, expect, it } from 'vitest';
import type { CommunityLeaderboard, CommunityRow } from '@ttac/shared/ratings';
import { columnTitle, countingColumns, editionFacts, editionLabel, editionToShow, parseCommunityView, titleMark, tournamentSums } from './community';

const row = (r: Partial<CommunityRow>): CommunityRow => ({
  rank: 1, name: 'ann', country: null, title: null, points: 0, events: 0, scores: {}, ...r,
});

const sampleSummary: CommunityLeaderboard['summary'] = [
  { id: 'ic', name: 'International Championship', short: 'IC', cols: ['ic_24', 'ic_25'], max_pts: 1000, badge_class: 'badge-gold', desc: 'IC' },
  { id: 'wrld', name: 'World Championship', short: 'WC', cols: ['wrld'], max_pts: 2500, badge_class: 'badge-gold', desc: 'WC' },
  { id: 'qd', name: 'Quick Draw', short: 'QD', cols: ['qd'], max_pts: 600, badge_class: 'badge-silver', desc: 'QD' },
];

const sampleLayout: Pick<CommunityLeaderboard, 'categories' | 'summary' | 'counting_events'> = {
  counting_events: 8,
  summary: sampleSummary,
  categories: [
    { category: 'World Championship', badge_class: 'badge-gold', columns: [{ id: 'wrld', name: 'World Championship', short: 'WC', max_pts: 2500 }] },
    { category: 'International Championship', badge_class: 'badge-gold', columns: [
      { id: 'ic_24', name: 'IC 24', short: 'IC24', max_pts: 1000 },
      { id: 'ic_25', name: 'IC 25', short: 'IC25', max_pts: 1000 },
      { id: 'ic_26', name: 'IC 26', short: 'IC26', max_pts: 1000 },
      { id: 'ic_27', name: 'IC 27', short: 'IC27', max_pts: 1000 },
    ]},
    { category: 'Invitational Cups', badge_class: 'badge-silver', columns: [
      { id: 'eif', name: 'Eiffel', short: 'EIF', max_pts: 1000 },
      { id: 'qd', name: 'Quick Draw', short: 'QD', max_pts: 600 },
    ]},
  ],
};

describe('parseCommunityView', () => {
  it.each([
    ['y2024', '2024'],
    ['race', 'race26'],
    ['1999', '1999'],
  ])("reads edition name %s as %s", (edition, want) => {
    expect(parseCommunityView(new URLSearchParams({ edition })).edition).toBe(want);
  });

  it.each([
    ['no edition', ''],
    ['the current one', 'edition=current'],
  ])('leaves the edition to admin with %s', (_, query) => {
    expect(parseCommunityView(new URLSearchParams(query)).edition).toBe('');
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

  it("respects a custom counting count (e.g. best 6 for Race 26)", () => {
    const scores = { a: 10, b: 50, c: 50, d: 1, e: 2, f: 3, g: 4, h: 5, i: 6, j: 0 };
    expect(countingColumns(scores, ['a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j'], 6)).toEqual(['b', 'c', 'a', 'i', 'h', 'g']);
  });
});

describe('tournamentSums', () => {
  it("sums each tournament's positive events and says whether one counts", () => {
    const got = tournamentSums({ ic_24: 1000, ic_25: 510, wrld: 0, qd: 20 }, sampleSummary, ['ic_25']);
    expect(got).toEqual({ wrld: null, ic: { sum: 1510, counting: true }, qd: { sum: 20, counting: false } });
  });
});

describe('titleMark', () => {
  it.each([
    ["WC for the edition's champion, over their title", { name: 'champion', title: 'GM' }, 'WC'],
    ['a Master', { title: 'M' }, 'M'],
    ['a Super GM', { title: 'SGM' }, 'SGM'],
    ['a Silver', { title: 'S' }, 'S'],
    ['a Wood', { title: 'W' }, 'W'],
    ['nothing for no title', { title: null }, null],
  ] as const)('shows %s', (_name, fields, want) => {
    expect(titleMark(row(fields), 'champion')).toBe(want);
  });
});

describe('editionFacts', () => {
  it("counts event slots, tiers and tournaments", () => {
    expect(editionFacts(sampleLayout)).toMatchObject({ counting: 8, slots: 7, tiers: 3, tournaments: 3 });
  });

  it("lists each tier with its own columns and points", () => {
    const tiers = editionFacts(sampleLayout).lines;
    expect(tiers[0]).toEqual({ name: 'World Championship', codes: 'wrld', maxPts: '2,500', each: false });
    expect(tiers[1]).toEqual({ name: 'International Championship', codes: 'ic_24–27', maxPts: '1,000', each: true });
    expect(tiers[2]).toEqual({ name: 'Invitational Cups', codes: 'eif, qd', maxPts: '600–1,000', each: true });
  });

  it('gives no maximum for a tier with a column whose maximum is unknown', () => {
    const unknown = {
      ...sampleLayout,
      categories: [{ category: 'Cups', badge_class: 'b', columns: [{ id: 'eif', name: 'E', short: 'E', max_pts: 1000 }, { id: 'qd', name: 'Q', short: 'Q', max_pts: null }] }],
    };
    expect(editionFacts(unknown).lines[0]?.maxPts).toBeNull();
  });
});

describe('columnTitle', () => {
  it.each([
    ['a known maximum', 2500, 'World Championship (Max: 2500 pts)'],
    ['an unknown one', null, 'World Championship'],
  ] as const)('names the column with %s', (_, max, want) => {
    expect(columnTitle({ id: 'wrld', name: 'World Championship', short: 'WC', max_pts: max })).toBe(want);
  });
});

describe('editionLabel', () => {
  it.each([
    ['its short name', { name: '2025 Final Archive', short_name: '2025 Final', badge: '🏆 2025 Final' }, '2025 Final'],
    ['its name without a short one', { name: '2025 Final Archive', short_name: '', badge: '🏆 2025 Final' }, '2025 Final Archive'],
  ] as const)('labels an edition with %s, once', (_, edition, want) => {
    expect(editionLabel(edition)).toBe(want);
  });
});

describe('editionToShow', () => {
  const editions = [{ slug: 'first' }, { slug: 'second' }];
  it.each([
    ['the edition asked for', 'second', editions, 'second'],
    ["admin's first edition when none is asked for", '', editions, 'first'],
    ['none when admin shows none', '', [], null],
  ] as const)('shows %s', (_, asked, list, want) => {
    expect(editionToShow(asked, list)).toBe(want);
  });
});
