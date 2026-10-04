/** Paging through a long table: which page a visitor's typed number means. */

/** typedPage returns the page typed, kept within 1 to pages; anything not a number is the first page. */
export function typedPage(typed: string, pages: number): number {
  const n = Number.parseInt(typed, 10);
  if (Number.isNaN(n)) return 1;
  return Math.min(Math.max(n, 1), Math.max(pages, 1));
}
