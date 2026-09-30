/** The site's sections, as the header nav lists them. A section is added here when it exists. */

export interface Section {
  label: string;
  href: string;
}

export const sections: Section[] = [
  { label: 'Ratings', href: '/ratings/' },
  { label: 'Hall of Fame', href: '/ratings/hall_of_fame/' },
  { label: 'Community Leaderboard', href: '/ratings/community/' },
  { label: 'Model Analysis', href: '/ratings/analysis/' },
  { label: 'FAQ & Guide', href: '/ratings/faq/' },
  { label: 'Methodology', href: '/ratings/methodology/' },
];

/** currentSection returns the href of the most specific section a path is in, or undefined. */
export function currentSection(path: string): string | undefined {
  const withSlash = path.endsWith('/') ? path : `${path}/`;
  return sections
    .map((s) => s.href)
    .filter((href) => withSlash.startsWith(href))
    .sort((a, b) => b.length - a.length)[0];
}
