import { describe, expect, it } from 'vitest';
import { EVENTS, endpointChanged, endpointSetting, endpointStatus, subscriptions, toggled, toggledFeed } from './notification-endpoints';

const SAVED = { name: 'Admins', url: '', webhook_id: '100000000000000001', events: ['link_request' as const], feeds: [2, 5], feed_options: [{ feed: 5, categories: [], keywords: ['tta'], was_live: true }], prefix: '', paused: false, forum: false, username: '', avatar_url: '', role_id: '', thread_id: '', blitz_reminder_hours: 24, colour: '#ff4500', text: 'full' as const, image: true };

describe('notification endpoints', () => {
  it('offers every event the API knows', () => {
    expect(EVENTS.map((e) => e.key)).toEqual(['sign_up', 'link_request', 'blitz_report', 'blitz_flag', 'feed_failing', 'ratings_run', 'ratings_top10', 'blitz_round', 'blitz_finished', 'blitz_reminder']);
  });

  it.each([
    ['unchanged', SAVED, false],
    ['renamed', { ...SAVED, name: 'Admin channel' }, true],
    ['given a new URL', { ...SAVED, url: 'https://discord.com/api/webhooks/100000000000000002/other' }, true],
    ['given a prefix', { ...SAVED, prefix: '[dev]' }, true],
    ['taking another event', { ...SAVED, events: ['link_request' as const, 'blitz_flag' as const] }, true],
    ['taking the same events in another order', { ...SAVED, events: [...SAVED.events] }, false],
    ['taking another feed', { ...SAVED, feeds: [2] }, true],
    ['paused', { ...SAVED, paused: true }, true],
    ['made a forum channel\'s', { ...SAVED, forum: true }, true],
    ['given a display name', { ...SAVED, username: 'TTA News' }, true],
    ['given a picture', { ...SAVED, avatar_url: 'https://example.com/a.png' }, true],
    ['given a role to ping', { ...SAVED, role_id: '100000000000000002' }, true],
    ['given a thread to post into', { ...SAVED, thread_id: '100000000000000021' }, true],
    ['reminding at other hours', { ...SAVED, blitz_reminder_hours: 6 }, true],
    ['given only spaces as a display name', { ...SAVED, username: '  ' }, false],
    ['given a colour', { ...SAVED, colour: '#123abc' }, true],
    ['showing less text', { ...SAVED, text: 'short' as const }, true],
    ['hiding the image', { ...SAVED, image: false }, true],
    ['taking the same feeds in another order', { ...SAVED, feeds: [5, 2] }, false],
    ['given another keyword', { ...SAVED, feed_options: [{ feed: 5, categories: [], keywords: ['tta', 'tournament'], was_live: true }] }, true],
    ['turning "was live" off', { ...SAVED, feed_options: [{ feed: 5, categories: [], keywords: ['tta'], was_live: false }] }, true],
    ['given the same options, written another way', { ...SAVED, feed_options: [{ was_live: true, match: 'all' as const, keywords: ['tta'], categories: [], feed: 5 }] }, false],
    ['given options for a feed it does not take', { ...SAVED, feed_options: [...SAVED.feed_options, { feed: 9, categories: ['Chess'], keywords: [], was_live: false }] }, false],
    ['left without a name', { ...SAVED, name: '  ' }, false],
  ])('can be saved when %s: %s', (_, draft, want) => {
    expect(endpointChanged(SAVED, draft)).toBe(want);
  });

  const NEW = { ...SAVED, name: '', webhook_id: '' };
  it.each([
    ['named, without a URL', { ...NEW, name: 'Admins' }, false],
    ['named, with a URL', { ...NEW, name: 'Admins', url: 'https://discord.com/api/webhooks/100000000000000002/token' }, true],
  ])('can be added when %s', (_, draft, want) => {
    expect(endpointChanged(NEW, draft)).toBe(want);
  });

  it.each([
    ['keeps the saved URL when none is given', { ...SAVED, url: '  ' }, undefined],
    ['sends a new URL', { ...SAVED, url: ' https://discord.com/api/webhooks/100000000000000002/token ' }, 'https://discord.com/api/webhooks/100000000000000002/token'],
  ])('a setting %s', (_, draft, want) => {
    const setting = endpointSetting(draft);
    expect(setting.url).toBe(want);
    expect('webhook_id' in setting).toBe(false);
  });

  it('sends the hours a reminder goes out before a deadline', () => {
    expect(endpointSetting({ ...SAVED, blitz_reminder_hours: 6 }).blitz_reminder_hours).toBe(6);
  });

  it('sends the thread it posts into, trimmed', () => {
    expect(endpointSetting({ ...SAVED, thread_id: ' 100000000000000021 ' }).thread_id).toBe('100000000000000021');
  });

  it('sends the options only of the feeds it takes', () => {
    const draft = { ...SAVED, feeds: [2], feed_options: [{ feed: 2, categories: ['Through the Ages'], keywords: [], was_live: false }, ...SAVED.feed_options] };
    expect(endpointSetting(draft).feed_options).toEqual([{ feed: 2, categories: ['Through the Ages'], keywords: [], match: 'all', was_live: false }]);
  });

  it.each([
    ['adding an event', ['blitz_flag' as const], 'link_request' as const, true, ['link_request', 'blitz_flag']],
    ['adding one already there', ['blitz_flag' as const], 'blitz_flag' as const, true, ['blitz_flag']],
    ['taking an event out', ['link_request' as const, 'blitz_flag' as const], 'link_request' as const, false, ['blitz_flag']],
  ])('toggles events: %s', (_, events, event, on, want) => {
    expect(toggled(events, event, on)).toEqual(want);
  });

  const FEEDS = [
    { kind: 'rss' as const, id: 2, name: 'Example news' },
    { kind: 'rss' as const, id: 5, name: 'Other news' },
    { kind: 'twitch' as const, id: 7, name: 'some_streamer' },
  ];

  it.each([
    ['nothing', [], [], 'Nothing: no posts reach it'],
    ['every event', ['sign_up', 'link_request', 'blitz_report', 'blitz_flag', 'feed_failing', 'ratings_run', 'ratings_top10', 'blitz_round', 'blitz_finished', 'blitz_reminder'] as const, [], 'every site event'],
    ['some events, in the tab\'s order', ['blitz_flag', 'sign_up'] as const, [], 'New players signing up, Blitz reports flagged'],
    ['an RSS feed', [], [5], 'Other news on RSS'],
    ['events and feeds', ['link_request'] as const, [2, 5], 'Requests to be linked; Example news on RSS; Other news on RSS'],
    ['a Twitch streamer', [], [7], 'some_streamer on Twitch'],
    ['a feed since removed', [], [9], 'Nothing: no posts reach it'],
  ])('says what an endpoint takes: %s', (_, events, feeds, want) => {
    expect(subscriptions(events, feeds, FEEDS)).toBe(want);
  });

  it.each([
    ['adding a feed', [5], 2, true, [2, 5]],
    ['adding one already there', [2], 2, true, [2]],
    ['taking one out', [2, 5], 2, false, [5]],
  ])('toggles feeds: %s', (_, feeds, feed, on, want) => {
    expect(toggledFeed(feeds, feed, on)).toEqual(want);
  });

  const when = (iso: string) => `[${iso}]`;
  const DELIVERY = { paused: false, last_sent_at: null, last_error: null, last_error_at: null };

  it.each([
    ['new', DELIVERY, 'Nothing sent yet'],
    ['sent', { ...DELIVERY, last_sent_at: '2026-11-17T01:00:00Z' }, 'Sent [2026-11-17T01:00:00Z]'],
    ['failing', { ...DELIVERY, last_sent_at: '2026-11-17T01:00:00Z', last_error: 'Discord answered 404', last_error_at: '2026-11-17T02:00:00Z' }, 'Failed [2026-11-17T02:00:00Z]: Discord answered 404'],
    ['paused', { ...DELIVERY, paused: true, last_sent_at: '2026-11-17T01:00:00Z' }, 'Paused: sent nothing'],
  ])('says how an endpoint is doing: %s', (_, e, want) => {
    expect(endpointStatus(e, when)).toBe(want);
  });
});
