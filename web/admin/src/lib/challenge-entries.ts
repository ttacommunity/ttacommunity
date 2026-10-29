/** Admin's list of challenge entries: how many pages it has, and what each entry's flags say. */

/** pageCount is how many pages total entries fill at size a page; an empty list is one empty page. */
export function pageCount(total: number, size: number): number {
  return Math.max(1, Math.ceil(total / size));
}

/** entryFlags says what admins have marked on an entry, and whether its proof was looked at. */
export function entryFlags(e: { hidden: boolean; hidden_reason: string; bugged: boolean; checked: boolean; proof_url: string | null }): string[] {
  const flags: string[] = [];
  if (e.hidden) flags.push(e.hidden_reason === '' ? 'Hidden' : `Hidden: ${e.hidden_reason}`);
  if (e.bugged) flags.push('Bugged');
  if (e.proof_url !== null) flags.push(e.checked ? 'Proof checked' : 'Proof not checked');
  return flags;
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
