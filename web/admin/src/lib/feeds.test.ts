import { describe, expect, it } from 'vitest';
import { feedGroups, feedRows, optionsFor, pauseRequest, removeRequest, rssName, rssStatus, twitchStatus, withOptions, words } from './feeds';

const when = (iso: string) => `at ${iso}`;
const twitch = (id: number, login: string, statuses: string[], paused = false) => ({
  id, login, display_name: login, user_id: '100000001', paused, created_at: '2026-11-18T08:00:00Z',
  subscriptions: statuses.map((status, i) => ({ type: i === 0 ? 'stream.online' : 'stream.offline', status })),
});
const rss = (id: number, title: string, url = 'https://news.example.com/feed.xml', paused = false) => ({
  id, url, title, paused, checked_at: '2026-11-18T08:00:00Z', last_item_at: '2026-11-18T07:00:00Z',
});

describe('feeds', () => {
  it('lists Twitch streamers then RSS feeds, each by name', () => {
    const rows = feedRows({
      twitch: [twitch(3, 'some_streamer', ['enabled', 'enabled'])],
      rss: [rss(5, 'Zebra news'), rss(4, 'Example news', 'https://blog.example.com/atom.xml')],
    }, when);
    expect(rows.map((r) => `${r.kind} ${r.name} ${r.href}`)).toEqual([
      'twitch some_streamer https://www.twitch.tv/some_streamer',
      'rss Example news https://blog.example.com/atom.xml',
      'rss Zebra news https://news.example.com/feed.xml',
    ]);
  });

  it.each([
    ['new', { paused: false }, 'Not read yet: its first read posts nothing'],
    ['read, no items', { paused: false, checked_at: '2026-11-17T01:00:00Z' }, 'Read at 2026-11-17T01:00:00Z'],
    ['read, with an item', { paused: false, checked_at: '2026-11-17T01:00:00Z', last_item_at: '2026-11-14T10:00:00Z' }, 'Read at 2026-11-17T01:00:00Z; newest item at 2026-11-14T10:00:00Z'],
    ['failing', { paused: false, checked_at: '2026-11-17T01:00:00Z', last_error: 'the feed answered 404', last_error_at: '2026-11-17T02:00:00Z' }, 'Failed at 2026-11-17T02:00:00Z: the feed answered 404'],
    ['paused', { paused: true, checked_at: '2026-11-17T01:00:00Z' }, 'Paused: not read'],
  ])('says how an RSS feed is doing when %s', (_, feed, want) => {
    expect(rssStatus(feed, when)).toBe(want);
  });

  it.each([
    ['its title', 'Example news', 'https://news.example.com/feed.xml', 'Example news'],
    ['its host before its first read', '', 'https://news.example.com/feed.xml', 'news.example.com'],
    ['its address when it is not one', '', 'not an address', 'not an address'],
  ])('names an RSS feed by %s', (_, title, url, want) => {
    expect(rssName({ title, url })).toBe(want);
  });

  it.each([
    ['a Twitch streamer', 'twitch' as const, false, '/api/twitch-feeds/7', { paused: true }],
    ['an RSS feed, resumed', 'rss' as const, true, '/api/rss-feeds/7', { paused: false }],
  ])('pauses or resumes %s', (_, kind, paused, path, json) => {
    expect(pauseRequest({ kind, id: 7, paused })).toEqual({ path, init: { method: 'PUT', json } });
  });

  it.each([
    ['twitch' as const, '/api/twitch-feeds/7'],
    ['rss' as const, '/api/rss-feeds/7'],
  ])('removes a %s feed', (kind, path) => {
    expect(removeRequest({ kind, id: 7 })).toEqual({ path, init: { method: 'DELETE', json: undefined } });
  });

  it.each([
    ['subscribed to both', ['enabled', 'enabled'], false, 'Subscribed'],
    ['waiting for Twitch to check our callback', ['webhook_callback_verification_pending', 'enabled'], false, 'Waiting for Twitch to check the callback'],
    ['a subscription revoked', ['enabled', 'user_removed'], false, 'Not subscribed: user_removed'],
    ['no subscription', [], false, 'Not subscribed: Twitch could not be reached; remove and add it again'],
    ['paused', ['enabled', 'enabled'], true, 'Paused: streams not posted'],
  ])('says how a Twitch streamer is doing when %s', (_, statuses, paused, want) => {
    expect(twitchStatus(twitch(3, 'some_streamer', statuses, paused))).toBe(want);
  });

  it('groups the feeds an endpoint may take by kind, leaving out a kind with none', () => {
    const groups = feedGroups({ twitch: [twitch(3, 'some_streamer', [], true)], rss: [rss(4, '')] });
    expect(groups.map((g) => `${g.label}: ${g.feeds.map((f) => `${f.name}${f.paused ? ' (paused)' : ''}`).join(', ')}`)).toEqual([
      'Twitch: some_streamer (paused)',
      'RSS: news.example.com',
    ]);
    expect(feedGroups({ twitch: [twitch(3, 'some_streamer', [])], rss: [] }).map((g) => g.kind)).toEqual(['twitch']);
  });

  it.each([
    ['a comma-separated list', 'Through the Ages, Board Games', ['Through the Ages', 'Board Games']],
    ['spaces and empty parts', '  tta ,, tournament , ', ['tta', 'tournament']],
    ['repeats', 'tta, tta', ['tta']],
    ['nothing', '   ', []],
  ])('reads words from %s', (_, typed, want) => {
    expect(words(typed)).toEqual(want);
  });

  it('gives a feed no options until some are set, and drops the entry when they are cleared', () => {
    expect(optionsFor([], 3)).toEqual({ feed: 3, categories: [], keywords: [], match: 'all', was_live: false });
    const set = withOptions([], 3, { keywords: ['tta'], was_live: true });
    expect(set).toEqual([{ feed: 3, categories: [], keywords: ['tta'], match: 'all', was_live: true }]);
    expect(withOptions(set, 3, { keywords: [], was_live: false })).toEqual([]);
    expect(withOptions([], 3, { match: 'any' })).toEqual([{ feed: 3, categories: [], keywords: [], match: 'any', was_live: false }]);
  });
});
