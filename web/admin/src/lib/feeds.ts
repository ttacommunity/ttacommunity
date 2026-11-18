/** The Notifications tab's feeds of every kind: one list of rows, a Twitch streamer's, an RSS feed's and YouTube Live's status, the feeds an endpoint may take by kind, and each one's options. */
import type { components } from '@ttac/shared/api';
import * as req from './admin-requests';

type TwitchFeed = Pick<components['schemas']['TwitchFeed'], 'id' | 'login' | 'paused' | 'subscriptions'>;
type RSSFeed = Pick<components['schemas']['RSSFeed'], 'id' | 'url' | 'title' | 'paused' | 'checked_at' | 'last_item_at' | 'last_error' | 'last_error_at'>;
type YouTubeFeed = Pick<components['schemas']['YouTubeFeed'], 'id' | 'paused' | 'channels' | 'waiting' | 'live'>;

/** Feeds are the feeds of each kind, as the API lists them. */
export interface Feeds {
  twitch: readonly TwitchFeed[];
  rss: readonly RSSFeed[];
  /** The YouTube Live feed, when it has been added: there is one at most. */
  youtube: readonly YouTubeFeed[];
}
type FeedOption = components['schemas']['FeedOption'];

/** Kind is a feed's source. */
export type Kind = 'twitch' | 'rss' | 'youtube';

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
export const KIND_LABELS: Record<Kind, string> = { twitch: 'Twitch', rss: 'RSS', youtube: 'YouTube Live' };

/** addableKinds are the kinds of feed the Add sheet offers, in its order: YouTube Live only while it is not there, as there is one. */
export function addableKinds(hasYouTube: boolean): Kind[] {
  return hasYouTube ? ['rss', 'twitch'] : ['rss', 'twitch', 'youtube'];
}

/** YOUTUBE_NAME names the YouTube Live feed, which takes every Movies channel. */
export const YOUTUBE_NAME = "Movies' channels";

/** youtubeStatus says how the YouTube Live feed is doing: paused, or how many channels it watches and their streams live and to come. */
export function youtubeStatus(f: YouTubeFeed): string {
  if (f.paused) return 'Paused: streams neither checked nor posted';
  const watching = `Watching ${f.channels} Movies channel${f.channels === 1 ? '' : 's'}`;
  const streams = [f.live > 0 ? `${f.live} live` : '', f.waiting > 0 ? `${f.waiting} waiting to start` : ''].filter((p) => p !== '');
  return streams.length === 0 ? watching : `${watching}; ${streams.join(', ')}`;
}

/** rssName is an RSS feed's name: its own title, or until its first read its address's host. */
export function rssName(f: Pick<RSSFeed, 'title' | 'url'>): string {
  if (f.title !== '') return f.title;
  try {
    return new URL(f.url).host;
  } catch {
    return f.url;
  }
}

/** twitchStatus says how a Twitch streamer is doing: paused, subscribed, waiting for Twitch's check, or why not subscribed. */
export function twitchStatus(f: TwitchFeed): string {
  if (f.paused) return 'Paused: streams not posted';
  if (f.subscriptions.length === 0) return 'Not subscribed: Twitch could not be reached; remove and add it again';
  const failed = f.subscriptions.find((s) => s.status !== 'enabled' && s.status !== 'webhook_callback_verification_pending');
  if (failed) return `Not subscribed: ${failed.status}`;
  if (f.subscriptions.some((s) => s.status !== 'enabled')) return 'Waiting for Twitch to check the callback';
  return 'Subscribed';
}

/** rssStatus says how an RSS feed is doing, its times written with when: paused, failing and why, read (with its newest item), or new. */
export function rssStatus(f: Pick<RSSFeed, 'paused' | 'checked_at' | 'last_item_at' | 'last_error' | 'last_error_at'>, when: (iso: string) => string): string {
  if (f.paused) return 'Paused: not read';
  if (f.last_error && f.last_error_at) return `Failed ${when(f.last_error_at)}: ${f.last_error}`;
  if (!f.checked_at) return 'Not read yet: its first read posts nothing';
  const newest = f.last_item_at ? `; newest item ${when(f.last_item_at)}` : '';
  return `Read ${when(f.checked_at)}${newest}`;
}

/** feedRows is every feed as the Feeds card lists it: Twitch's then RSS's, each by name, then YouTube Live, times written with when. */
export function feedRows({ twitch, rss, youtube }: Feeds, when: (iso: string) => string): FeedRow[] {
  const byName = (a: FeedRow, b: FeedRow) => a.name.localeCompare(b.name);
  return [
    ...twitch.map((f): FeedRow => ({
      kind: 'twitch', id: f.id, name: f.login, href: `https://www.twitch.tv/${encodeURIComponent(f.login)}`,
      sends: 'streams going live', status: twitchStatus(f), paused: f.paused,
    })).sort(byName),
    ...rss.map((f): FeedRow => ({
      kind: 'rss', id: f.id, name: rssName(f), href: f.url, sends: 'new items',
      status: rssStatus(f, when), paused: f.paused,
    })).sort(byName),
    ...youtube.map((f): FeedRow => ({
      kind: 'youtube', id: f.id, name: YOUTUBE_NAME, href: '#movies', sends: 'live streams starting', status: youtubeStatus(f), paused: f.paused,
    })),
  ];
}

/** feedGroups is the feeds an endpoint may take, by kind, each by name; a kind with no feed is left out. */
export function feedGroups({ twitch, rss, youtube }: Feeds): { kind: Kind; label: string; feeds: FeedChoice[] }[] {
  const groups: { kind: Kind; label: string; feeds: FeedChoice[] }[] = [
    { kind: 'twitch', label: KIND_LABELS.twitch, feeds: twitch.map((f) => ({ kind: 'twitch' as const, id: f.id, name: f.login, paused: f.paused })) },
    { kind: 'rss', label: KIND_LABELS.rss, feeds: rss.map((f) => ({ kind: 'rss' as const, id: f.id, name: rssName(f), paused: f.paused })) },
    { kind: 'youtube', label: KIND_LABELS.youtube, feeds: youtube.map((f) => ({ kind: 'youtube' as const, id: f.id, name: YOUTUBE_NAME, paused: f.paused })) },
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

/** pauseRequest is the request that pauses row's feed, or resumes it when paused. */
export function pauseRequest(row: Pick<FeedRow, 'kind' | 'id' | 'paused'>) {
  const set = { twitch: req.setTwitchFeed, rss: req.setRSSFeed, youtube: req.setYouTubeFeed }[row.kind];
  return set(row.id, !row.paused);
}

/** removeRequest is the request that removes row's feed. */
export function removeRequest(row: Pick<FeedRow, 'kind' | 'id'>) {
  const remove = { twitch: req.removeTwitchFeed, rss: req.removeRSSFeed, youtube: req.removeYouTubeFeed }[row.kind];
  return remove(row.id);
}

/** resumedText says what resuming row's feed does: a streamer's next stream is posted, a feed's items from its next read on, YouTube's streams pushed from now on. */
export function resumedText(row: Pick<FeedRow, 'kind' | 'name'>): string {
  if (row.kind === 'twitch') return `Resumed ${row.name}: their next stream is posted.`;
  if (row.kind === 'youtube') return `Resumed ${row.name}: streams YouTube tells of from now on are posted.`;
  return `Resumed ${row.name}: posts from its next read on are posted.`;
}
