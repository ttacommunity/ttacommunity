/** The site's sections and their pages, the one list the header, its phone menu and the breadcrumb are built from. */

export interface NavPage {
  label: string;
  href: string;
  /** One line under the label in the section's menu. */
  description: string;
}

/** A section opens a menu of its pages; one without pages, Home, is a plain link. */
export interface NavSection {
  label: string;
  href?: string;
  pages: NavPage[];
}

/** sections is every section in the header's order. A page is added when it exists. */
export const sections: readonly NavSection[] = [
  { label: 'Home', href: '/', pages: [] },
  {
    label: 'Ratings',
    pages: [
      { label: 'Leaderboard', href: '/ratings/', description: 'Every rated player, by format' },
      { label: 'Model analysis', href: '/ratings/analysis/', description: 'How well the ratings predict results' },
      { label: 'Methodology', href: '/ratings/methodology/', description: 'How the ratings are worked out' },
      { label: 'FAQ & guide', href: '/ratings/faq/', description: 'The questions players ask' },
    ],
  },
  {
    label: 'Tournaments',
    pages: [
      { label: 'All tournaments', href: '/ratings/tournaments/', description: 'Every tournament on record' },
      { label: 'Hall of Fame', href: '/ratings/hall_of_fame/', description: 'Champions and podiums, series by series' },
      { label: 'Community leaderboard', href: '/ratings/community/', description: 'Standings under the Best-8 system' },
      { label: 'Blitz', href: '/blitz/', description: 'Weekly pairings, results and standings' },
    ],
  },
  {
    label: 'Challenges',
    pages: [{ label: 'Highscores', href: '/challenges/', description: 'Every challenge and its record, by set' }],
  },
];

/** Place is where a path sits: its section, and the menu page it is or is below (none for Home). */
export interface Place {
  section: NavSection;
  page?: NavPage;
}

/** Crumb is one step of a breadcrumb: a link, a section's name (no href), or the page itself (current). */
export interface Crumb {
  label: string;
  href?: string;
  current?: true;
}

function withSlash(path: string): string {
  return path.endsWith('/') ? path : `${path}/`;
}

/** placeOf returns the section and the most specific menu page a path is in, or undefined outside every section. */
export function placeOf(path: string): Place | undefined {
  const p = withSlash(path);
  if (p === '/') return { section: sections[0] };
  let best: Place | undefined;
  for (const section of sections) {
    for (const page of section.pages) {
      if (p.startsWith(page.href) && (best?.page === undefined || page.href.length > best.page.href.length)) best = { section, page };
    }
  }
  return best;
}

/** breadcrumb returns the trail from Home to a path's menu page, the page itself marked current; empty at Home and outside the sections. */
export function breadcrumb(path: string): Crumb[] {
  const place = placeOf(path);
  if (place?.page === undefined) return [];
  const page = withSlash(path) === place.page.href ? { label: place.page.label, current: true as const } : { label: place.page.label, href: place.page.href };
  return [{ label: 'Home', href: '/' }, { label: place.section.label }, page];
}
