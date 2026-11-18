/** The Notifications tab's feeds of every kind: one list of rows, a Twitch streamer's status, the feeds an endpoint may take by kind, and each one's options. */
import type { components } from '@ttac/shared/api';
import { feedSends, feedStatus } from './reddit-feeds';

type RedditFeed = Pick<components['schemas']['RedditFeed'], 'id' | 'subreddit' | 'paused' | 'kinds' | 'checked_at' | 'last_post_at' | 'last_error' | 'last_error_at'>;
type TwitchFeed = Pick<components['schemas']['TwitchFeed'], 'id' | 'login' | 'paused' | 'subscriptions'>;
type FeedOption = components['schemas']['FeedOption'];

/** Kind is a feed's source. */
export type Kind = 'reddit' | 'twitch';

/** FeedRow is a feed as the Feeds card lists it. */
export interface FeedRow {
  kind: Kind;
  id: number;
  name: string;
  href: string;
  sends: string;
  status: string;
  paused: boolean;
}

/** FeedChoice is a feed as an endpoint's sheet offers it. */
export interface FeedChoice {
  kind: Kind;
  id: number;
  name: string;
  paused: boolean;
}

/** KIND_LABELS name each kind of feed. */
export const KIND_LABELS: Record<Kind, string> = { reddit: 'Reddit', twitch: 'Twitch' };

const redditName = (f: Pick<RedditFeed, 'subreddit'>) => `r/${f.subreddit}`;

/** twitchStatus says how a Twitch streamer is doing: paused, subscribed, waiting for Twitch's check, or why not subscribed. */
export function twitchStatus(f: TwitchFeed): string {
  if (f.paused) return 'Paused: streams not posted';
  if (f.subscriptions.length === 0) return 'Not subscribed: Twitch could not be reached; remove and add it again';
  const failed = f.subscriptions.find((s) => s.status !== 'enabled' && s.status !== 'webhook_callback_verification_pending');
  if (failed) return `Not subscribed: ${failed.status}`;
  if (f.subscriptions.some((s) => s.status !== 'enabled')) return 'Waiting for Twitch to check the callback';
  return 'Subscribed';
}

/** feedRows is every feed as the Feeds card lists it: Reddit's then Twitch's, each by name, times written with when. */
export function feedRows(reddit: readonly RedditFeed[], twitch: readonly TwitchFeed[], when: (iso: string) => string): FeedRow[] {
  const byName = (a: FeedRow, b: FeedRow) => a.name.localeCompare(b.name);
  return [
    ...reddit.map((f): FeedRow => ({
      kind: 'reddit', id: f.id, name: redditName(f), href: `https://www.reddit.com/r/${encodeURIComponent(f.subreddit)}/new/`,
      sends: feedSends(f.kinds), status: feedStatus(f, when), paused: f.paused,
    })).sort(byName),
    ...twitch.map((f): FeedRow => ({
      kind: 'twitch', id: f.id, name: f.login, href: `https://www.twitch.tv/${encodeURIComponent(f.login)}`,
      sends: 'streams going live', status: twitchStatus(f), paused: f.paused,
    })).sort(byName),
  ];
}

/** feedGroups is the feeds an endpoint may take, by kind, each by name; a kind with no feed is left out. */
export function feedGroups(reddit: readonly RedditFeed[], twitch: readonly TwitchFeed[]): { kind: Kind; label: string; feeds: FeedChoice[] }[] {
  const groups: { kind: Kind; label: string; feeds: FeedChoice[] }[] = [
    { kind: 'reddit', label: KIND_LABELS.reddit, feeds: reddit.map((f) => ({ kind: 'reddit' as const, id: f.id, name: redditName(f), paused: f.paused })) },
    { kind: 'twitch', label: KIND_LABELS.twitch, feeds: twitch.map((f) => ({ kind: 'twitch' as const, id: f.id, name: f.login, paused: f.paused })) },
  ];
  return groups.filter((g) => g.feeds.length > 0).map((g) => ({ ...g, feeds: g.feeds.sort((a, b) => a.name.localeCompare(b.name)) }));
}

/** words reads a comma-separated list: each part trimmed, without empty ones or repeats. */
export function words(typed: string): string[] {
  const out: string[] = [];
  for (const w of typed.split(',').map((p) => p.trim())) if (w !== '' && !out.includes(w)) out.push(w);
  return out;
}

/** optionsFor is feed's options among opts, or none set; match is all unless any is set. */
export function optionsFor(opts: readonly FeedOption[], feed: number): FeedOption {
  const o = opts.find((x) => x.feed === feed) ?? { feed, categories: [], keywords: [], was_live: false };
  return { ...o, match: o.match ?? 'all' };
}

/** withOptions returns opts with feed's options changed by change; options with nothing set are left out. */
export function withOptions(opts: readonly FeedOption[], feed: number, change: Partial<Omit<FeedOption, 'feed'>>): FeedOption[] {
  const next = { ...optionsFor(opts, feed), ...change };
  const rest = opts.filter((o) => o.feed !== feed);
  const empty = next.categories.length === 0 && next.keywords.length === 0 && next.match === 'all' && !next.was_live;
  return (empty ? rest : [...rest, next]).sort((a, b) => a.feed - b.feed);
}
