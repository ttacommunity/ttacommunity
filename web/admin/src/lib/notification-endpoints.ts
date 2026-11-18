/** The Notifications tab's decisions: the events and feeds an endpoint may take, what it takes in a line, whether an edit can be saved, and what is sent. */
import type { components } from '@ttac/shared/api';
import { KIND_LABELS, type FeedChoice } from './feeds';

type Event = components['schemas']['NotificationEvent'];
type Feed = Pick<FeedChoice, 'kind' | 'id' | 'name'>;
type FeedOption = components['schemas']['FeedOption'];
type Setting = Pick<
  components['schemas']['NotificationEndpoint'],
  'name' | 'webhook_id' | 'prefix' | 'paused' | 'forum' | 'username' | 'avatar_url' | 'role_id' | 'colour' | 'text' | 'image' | 'author'
> & {
  /** A new webhook URL, or '' to keep the saved one: the API never sends it back. */
  url: string;
  events: readonly Event[];
  feeds: readonly number[];
  feed_options: readonly FeedOption[];
};
type Delivery = Pick<components['schemas']['NotificationEndpoint'], 'paused' | 'last_sent_at' | 'last_error' | 'last_error_at'>;

/** TEXTS are how much of a Reddit post's text an endpoint may show. */
export const TEXTS: { key: components['schemas']['RedditPostText']; label: string }[] = [
  { key: 'full', label: 'All of it' },
  { key: 'short', label: 'The first few lines' },
  { key: 'none', label: 'None: title only' },
];

/** EVENTS are the events an endpoint may take, in the order the tab lists them. */
export const EVENTS: { key: Event; label: string }[] = [
  { key: 'sign_up', label: 'New players signing up' },
  { key: 'link_request', label: 'Requests to be linked' },
  { key: 'blitz_report', label: 'Blitz results reported or corrected' },
  { key: 'blitz_flag', label: 'Blitz reports flagged' },
  { key: 'reddit_failing', label: 'Reddit feeds failing or reading again' },
];

/** endpointChanged says whether an endpoint's edit can be saved: it has a name, a URL given or saved, and differs from what is saved. */
export function endpointChanged(saved: Setting, draft: Setting): boolean {
  if (draft.name.trim() === '' || (draft.url.trim() === '' && saved.webhook_id === '')) return false;
  const texts = ['name', 'url', 'prefix', 'username', 'avatar_url', 'role_id'] as const;
  const choices = ['paused', 'forum', 'colour', 'text', 'image', 'author'] as const;
  return texts.some((k) => draft[k].trim() !== saved[k]) || choices.some((k) => draft[k] !== saved[k]) ||
    !sameSet(saved.events, draft.events) || !sameSet(saved.feeds, draft.feeds) ||
    JSON.stringify(takenOptions(saved)) !== JSON.stringify(takenOptions(draft));
}

/** takenOptions is s's feed options for the feeds it takes, by feed, each in one shape so two can be compared. */
function takenOptions(s: Pick<Setting, 'feeds' | 'feed_options'>): FeedOption[] {
  return s.feed_options
    .filter((o) => s.feeds.includes(o.feed))
    .map((o) => ({ feed: o.feed, categories: o.categories, keywords: o.keywords, match: o.match ?? 'all', was_live: o.was_live }))
    .sort((a, b) => a.feed - b.feed);
}

/** endpointSetting is what saving draft sends: without the webhook's id, and without a URL when none was given, keeping the saved one. */
export function endpointSetting(draft: Setting): Omit<components['schemas']['NotificationEndpointSetting'], 'format'> {
  const { name, prefix, paused, forum, username, avatar_url, role_id, colour, text, image, author } = draft;
  const setting = { name, prefix, paused, forum, username, avatar_url, role_id, colour, text, image, author, events: [...draft.events], feeds: [...draft.feeds],
    feed_options: takenOptions(draft) };
  return draft.url.trim() === '' ? setting : { ...setting, url: draft.url.trim() };
}

/** endpointStatus says how an endpoint's posts are going, its times written with when: paused, failing and why, sent, or nothing yet. */
export function endpointStatus(e: Delivery, when: (iso: string) => string): string {
  if (e.paused) return 'Paused: sent nothing';
  if (e.last_error && e.last_error_at) return `Failed ${when(e.last_error_at)}: ${e.last_error}`;
  if (e.last_sent_at) return `Sent ${when(e.last_sent_at)}`;
  return 'Nothing sent yet';
}

/** sameSet says whether a and b hold the same items, in any order. */
function sameSet<T>(a: readonly T[], b: readonly T[]): boolean {
  return a.length === b.length && a.every((x) => b.includes(x));
}

/** subscriptions says in a line what an endpoint takes, from the feeds there are: its site events, then its feeds by name. */
export function subscriptions(events: readonly Event[], feeds: readonly number[], all: readonly Feed[]): string {
  const parts: string[] = [];
  if (events.length === EVENTS.length) parts.push('every site event');
  else if (events.length > 0) parts.push(EVENTS.filter((e) => events.includes(e.key)).map((e) => e.label).join(', '));
  parts.push(...all.filter((f) => feeds.includes(f.id)).map((f) => (f.kind === 'reddit' ? f.name : `${f.name} on ${KIND_LABELS[f.kind]}`)));
  return parts.length === 0 ? 'Nothing: no posts reach it' : parts.join('; ');
}

/** toggledFeed returns feeds with feed taken (on) or not, by id. */
export function toggledFeed(feeds: readonly number[], feed: number, on: boolean): number[] {
  const rest = feeds.filter((f) => f !== feed);
  return (on ? [...rest, feed] : rest).sort((a, b) => a - b);
}

/** toggled returns events with event taken (on) or not, in EVENTS's order. */
export function toggled(events: readonly Event[], event: Event, on: boolean): Event[] {
  return EVENTS.map((e) => e.key).filter((k) => (k === event ? on : events.includes(k)));
}
