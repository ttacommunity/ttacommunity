/** The site's two areas and the sections each one's header lists: the ratings (upstream's pages) and the root site around them. */

export interface Section {
  label: string;
  href: string;
}

/** Area is where a page sits: in the ratings, a copy of upstream's site, or on the root site, which leads to the ratings and to Blitz. */
export type Area = 'ratings' | 'hub';

const ratingsSections: Section[] = [
  { label: 'Ratings', href: '/ratings/' },
  { label: 'Hall of Fame', href: '/ratings/hall_of_fame/' },
  { label: 'Community Leaderboard', href: '/ratings/community/' },
  { label: 'Model Analysis', href: '/ratings/analysis/' },
  { label: 'FAQ & Guide', href: '/ratings/faq/' },
  { label: 'Methodology', href: '/ratings/methodology/' },
];

const hubSections: Section[] = [
  { label: 'Ratings', href: '/ratings/' },
  { label: 'Blitz', href: '/blitz/' },
];

/** sectionsFor returns the sections an area's header lists. A section is added when it exists. */
export function sectionsFor(area: Area): Section[] {
  return area === 'ratings' ? ratingsSections : hubSections;
}

function withSlash(path: string): string {
  return path.endsWith('/') ? path : `${path}/`;
}

/** areaOf says which area a path is in: everything under /ratings/ is the ratings, the rest the root site. */
export function areaOf(path: string): Area {
  return withSlash(path).startsWith('/ratings/') ? 'ratings' : 'hub';
}

/** currentSection returns the href of the most specific section of the path's area that the path is in, or undefined. */
export function currentSection(path: string): string | undefined {
  return sectionsFor(areaOf(path))
    .map((s) => s.href)
    .filter((href) => withSlash(path).startsWith(href))
    .sort((a, b) => b.length - a.length)[0];
}
