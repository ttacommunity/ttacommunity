/** The Notifications tab's Reddit feeds: how each is doing, the kinds of post it may send, and whether an edit can be saved. */
import type { components } from '@ttac/shared/api';

type Feed = Pick<components['schemas']['RedditFeed'], 'paused' | 'checked_at' | 'last_post_at' | 'last_error' | 'last_error_at'>;
type Kind = components['schemas']['RedditPostKind'];

/** KINDS are the kinds of post a feed may send, in the order the sheet lists them. */
export const KINDS: { key: Kind; label: string }[] = [
  { key: 'text', label: 'Text' },
  { key: 'image', label: 'Images' },
  { key: 'gallery', label: 'Galleries' },
  { key: 'video', label: 'Videos' },
  { key: 'link', label: 'Links' },
];

/** feedSends says in a line which kinds of post a feed sends. */
export function feedSends(kinds: readonly Kind[]): string {
  if (kinds.length === KINDS.length) return 'every kind';
  return KINDS.filter((k) => kinds.includes(k.key)).map((k) => k.label.toLowerCase()).join(', ') || 'nothing';
}

/** kindsChanged says whether a feed's kinds as edited differ from its saved ones, and may be saved: it sends some kind. */
export function kindsChanged(saved: readonly Kind[], draft: readonly Kind[]): boolean {
  return draft.length > 0 && !(saved.length === draft.length && saved.every((k) => draft.includes(k)));
}

/** toggledKind returns kinds with kind sent (on) or not, in KINDS's order. */
export function toggledKind(kinds: readonly Kind[], kind: Kind, on: boolean): Kind[] {
  return KINDS.map((k) => k.key).filter((k) => (k === kind ? on : kinds.includes(k)));
}

/** feedStatus says how a feed is doing, its times written with when: paused, failing and why, read (with its newest item, a post unless named), or new. */
export function feedStatus(f: Feed, when: (iso: string) => string, item = 'post'): string {
  if (f.paused) return 'Paused: not read';
  if (f.last_error && f.last_error_at) return `Failed ${when(f.last_error_at)}: ${f.last_error}`;
  if (!f.checked_at) return 'Not read yet: its first read posts nothing';
  const newest = f.last_post_at ? `; newest ${item} ${when(f.last_post_at)}` : '';
  return `Read ${when(f.checked_at)}${newest}`;
}
