import { describe, expect, it } from 'vitest';
import { currentSection, sections } from './nav';

describe('sections', () => {
  it("lists only the sections that exist, in upstream's menu order", () => {
    expect(sections.map((s) => s.href)).toEqual(['/ratings/', '/ratings/analysis/']);
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
  });

  it('matches nothing outside the sections or on a shared prefix', () => {
    expect(currentSection('/')).toBeUndefined();
    expect(currentSection('/ratingsx/')).toBeUndefined();
    expect(currentSection('/design/')).toBeUndefined();
  });
});
