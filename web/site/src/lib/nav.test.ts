import { describe, expect, it } from 'vitest';
import { areaOf, currentSection, sectionsFor } from './nav';

describe('sectionsFor', () => {
  it("lists the ratings' pages in upstream's menu order, and nothing outside the ratings", () => {
    expect(sectionsFor('ratings').map((s) => s.href)).toEqual([
      '/ratings/',
      '/ratings/hall_of_fame/',
      '/ratings/community/',
      '/ratings/analysis/',
      '/ratings/faq/',
      '/ratings/methodology/',
    ]);
  });
  it("lists the root site's sections, Blitz among them", () => {
    expect(sectionsFor('hub').map((s) => s.href)).toEqual(['/ratings/', '/blitz/']);
  });
});

describe('areaOf', () => {
  it.each([
    ['the ratings', '/ratings/', 'ratings'],
    ['the ratings without the slash', '/ratings', 'ratings'],
    ['a ratings page', '/ratings/hall_of_fame/royal_league/', 'ratings'],
    ['the home page', '/', 'hub'],
    ['blitz', '/blitz/', 'hub'],
    ['a shared prefix', '/ratingsx/', 'hub'],
    ['the design page', '/design/', 'hub'],
  ])('puts %s in its area', (_name, path, want) => {
    expect(areaOf(path)).toBe(want);
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
    ['the blitz section, on the root site', '/blitz/', '/blitz/'],
    ['nothing at the home page', '/', undefined],
    ['nothing on a shared prefix', '/ratingsx/', undefined],
    ['nothing outside the sections', '/design/', undefined],
  ])('matches %s', (_name, path, want) => {
    expect(currentSection(path)).toBe(want);
  });
});
