/** Admin's list of challenge entries: its views and their addresses, and which entries the text typed matches. */
import { matchRank } from './match';

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

/** entriesPath is the address of a view's entries. */
export function entriesPath(view: View): string {
  return view === 'all' ? '/api/challenges/entries' : `/api/challenges/entries?view=${view}`;
}

/** entriesMatching keeps, in their order, the entries matching any of the comma-separated terms typed: by player, challenge id or challenge name as matchRank matches; every entry for no text. */
export function entriesMatching<E extends { player: string; challenge: string; challenge_name: string }>(entries: readonly E[], typed: string): E[] {
  const terms = typed
    .split(',')
    .map((t) => t.trim())
    .filter((t) => t !== '');
  if (terms.length === 0) return [...entries];
  const matches = (e: E, t: string) => [e.player, e.challenge, e.challenge_name].some((f) => f !== '' && matchRank(f, t) !== null);
  return entries.filter((e) => terms.some((t) => matches(e, t)));
}
