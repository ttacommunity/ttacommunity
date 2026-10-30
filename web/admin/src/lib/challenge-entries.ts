/** Admin's list of challenge entries: how many pages it has, its views and their addresses. */

/** pageCount is how many pages total entries fill at size a page; an empty list is one empty page. */
export function pageCount(total: number, size: number): number {
  return Math.max(1, Math.ceil(total / size));
}

/** View is one of admin's views of the entries. */
export type View = 'all' | 'unchecked' | 'checked' | 'unverified' | 'flagged' | 'bugged' | 'hidden';

const VIEW_LABELS: Record<View, string> = {
  all: 'All entries',
  unchecked: 'Verified, not checked yet',
  checked: 'Verified, checked',
  unverified: 'Unverified',
  flagged: 'Flagged',
  bugged: 'Bug',
  hidden: 'Hidden',
};

/** viewLabel names a view as its tab does, with how many entries it holds. */
export function viewLabel(view: View, entries: number): string {
  return `${VIEW_LABELS[view]} (${entries})`;
}

/** entriesPath is the address of a page of a view's entries, narrowed to q when it is not blank. */
export function entriesPath(page: number, view: View, q: string): string {
  const params = new URLSearchParams({ page: String(page) });
  if (view !== 'all') params.set('view', view);
  if (q.trim() !== '') params.set('q', q.trim());
  return `/api/challenges/entries?${params}`;
}
