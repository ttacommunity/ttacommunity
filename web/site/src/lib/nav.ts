/** The site's sections, as the header nav lists them. A section is added here when it exists. */

export interface Section {
  label: string;
  href: string;
}

export const sections: Section[] = [{ label: 'Ratings', href: '/ratings/' }];

/** isCurrent reports whether a path is the section at href or a page under it. */
export function isCurrent(path: string, href: string): boolean {
  const withSlash = path.endsWith('/') ? path : `${path}/`;
  return withSlash.startsWith(href);
}
