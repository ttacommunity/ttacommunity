import { describe, expect, it } from 'vitest';
import { breadcrumb, placeOf, sections } from './nav';

describe('sections', () => {
  it('lists Home, then the sections with pages', () => {
    expect(sections.map((s) => s.label)).toEqual(['Home', 'Ratings', 'Tournaments']);
  });
  it('gives every page a description', () => {
    const pages = sections.flatMap((s) => s.pages);
    expect(pages.filter((p) => p.description === '')).toEqual([]);
  });
  it('lists each page once', () => {
    const hrefs = sections.flatMap((s) => s.pages.map((p) => p.href));
    expect(new Set(hrefs).size).toBe(hrefs.length);
  });
});

describe('placeOf', () => {
  it.each([
    ['the home page', '/', 'Home', undefined],
    ['the leaderboard', '/ratings/', 'Ratings', 'Leaderboard'],
    ['the leaderboard without its slash', '/ratings', 'Ratings', 'Leaderboard'],
    ['a player page', '/ratings/player/', 'Ratings', 'Leaderboard'],
    ["a player's achievements", '/ratings/player/achievements/', 'Ratings', 'Leaderboard'],
    ['model analysis', '/ratings/analysis/', 'Ratings', 'Model analysis'],
    ["a tab of model analysis", '/ratings/analysis/seats/', 'Ratings', 'Model analysis'],
    ['methodology', '/ratings/methodology/', 'Ratings', 'Methodology'],
    ['the FAQ', '/ratings/faq/', 'Ratings', 'FAQ & guide'],
    ['all tournaments, under the ratings path', '/ratings/tournaments/', 'Tournaments', 'All tournaments'],
    ['the Hall of Fame, under the ratings path', '/ratings/hall_of_fame/', 'Tournaments', 'Hall of Fame'],
    ['a Hall of Fame series', '/ratings/hall_of_fame/royal_league/', 'Tournaments', 'Hall of Fame'],
    ['the community leaderboard', '/ratings/community/', 'Tournaments', 'Community leaderboard'],
    ['blitz', '/blitz/', 'Tournaments', 'Blitz'],
  ])('puts %s in its section and page', (_name, path, section, page) => {
    const place = placeOf(path);
    expect([place?.section.label, place?.page?.label]).toEqual([section, page]);
  });

  it.each([
    ['the account page', '/account/'],
    ['privacy', '/privacy/'],
    ['the design page', '/design/'],
    ['a shared prefix', '/ratingsx/'],
    ['another shared prefix', '/blitzy/'],
  ])('puts %s nowhere', (_name, path) => {
    expect(placeOf(path)).toBeUndefined();
  });
});

describe('breadcrumb', () => {
  const home = { label: 'Home', href: '/' };
  it.each([
    ['a menu page', '/ratings/methodology/', [home, { label: 'Ratings' }, { label: 'Methodology', current: true }]],
    ['a menu page without its slash', '/blitz', [home, { label: 'Tournaments' }, { label: 'Blitz', current: true }]],
    ['a page below a menu page', '/ratings/player/', [home, { label: 'Ratings' }, { label: 'Leaderboard', href: '/ratings/' }]],
    [
      'a tab of a menu page',
      '/ratings/hall_of_fame/royal_league/',
      [home, { label: 'Tournaments' }, { label: 'Hall of Fame', href: '/ratings/hall_of_fame/' }],
    ],
  ])('leads from Home to %s', (_name, path, want) => {
    expect(breadcrumb(path)).toEqual(want);
  });

  it.each([
    ['the home page', '/'],
    ['a page in no section', '/account/'],
  ])('is empty on %s', (_name, path) => {
    expect(breadcrumb(path)).toEqual([]);
  });
});
