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
