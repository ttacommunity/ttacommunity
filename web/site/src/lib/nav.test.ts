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
  it('matches a section and every page under it', () => {
    expect(currentSection('/ratings/')).toBe('/ratings/');
    expect(currentSection('/ratings')).toBe('/ratings/');
    expect(currentSection('/ratings/player/x/')).toBe('/ratings/');
  });

  it('prefers the most specific section', () => {
    expect(currentSection('/ratings/analysis/')).toBe('/ratings/analysis/');
    expect(currentSection('/ratings/analysis/movers/')).toBe('/ratings/analysis/');
    expect(currentSection('/ratings/faq/')).toBe('/ratings/faq/');
    expect(currentSection('/ratings/hall_of_fame/royal_league/')).toBe('/ratings/hall_of_fame/');
  });

  it('matches nothing outside the sections or on a shared prefix', () => {
    expect(currentSection('/')).toBeUndefined();
    expect(currentSection('/ratingsx/')).toBeUndefined();
    expect(currentSection('/design/')).toBeUndefined();
  });
});
