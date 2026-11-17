/** The Notifications tab's Reddit feeds: how each is doing. */
import type { components } from '@ttac/shared/api';

type Feed = Pick<components['schemas']['RedditFeed'], 'paused' | 'checked_at' | 'last_post_at' | 'last_error' | 'last_error_at'>;

/** feedStatus says how a feed is doing, its times written with when: paused, failing and why, read, or new. */
export function feedStatus(f: Feed, when: (iso: string) => string): string {
  if (f.paused) return 'Paused: not read';
  if (f.last_error && f.last_error_at) return `Failed ${when(f.last_error_at)}: ${f.last_error}`;
  if (!f.checked_at) return 'Not read yet: its first read posts nothing';
  const newest = f.last_post_at ? `; newest post ${when(f.last_post_at)}` : '';
  return `Read ${when(f.checked_at)}${newest}`;
}
