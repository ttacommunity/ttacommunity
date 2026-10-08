import { describe, expect, it } from 'vitest';
import { currentSection, sections } from './nav';

describe('sections', () => {
  it("lists only the sections that exist, in upstream's menu order", () => {
    expect(sections.map((s) => s.href)).toEqual([
      '/ratings/',
      '/ratings/hall_of_fame/',
      '/ratings/community/',
      '/ratings/analysis/',
      '/ratings/faq/',
      '/ratings/methodology/',
    ]);
    expect(sections[1]?.label).toBe('Hall of Fame');
    expect(sections[2]?.label).toBe('Community Leaderboard');
  });
});

describe('currentSection', () => {
  it.each([
    ['a section', '/ratings/', '/ratings/'],
    ['a section without its slash', '/ratings', '/ratings/'],
    ['a page under a section', '/ratings/player/x/', '/ratings/'],
    ['the most specific section', '/ratings/analysis/', '/ratings/analysis/'],
    ['a page under the most specific section', '/ratings/analysis/movers/', '/ratings/analysis/'],
    ['another specific section', '/ratings/faq/', '/ratings/faq/'],
    ['a page under another specific section', '/ratings/hall_of_fame/royal_league/', '/ratings/hall_of_fame/'],
    ['nothing at the home page', '/', undefined],
    ['nothing on a shared prefix', '/ratingsx/', undefined],
    ['nothing outside the sections', '/design/', undefined],
  ])('matches %s', (_name, path, want) => {
    expect(currentSection(path)).toBe(want);
  });
});
