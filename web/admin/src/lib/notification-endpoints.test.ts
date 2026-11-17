import { describe, expect, it } from 'vitest';
import { EVENTS, endpointChanged, endpointStatus, subscriptions, toggled, toggledFeed } from './notification-endpoints';

const SAVED = { name: 'Admins', url: 'https://discord.com/api/webhooks/100000000000000001/token', events: ['link_request' as const], feeds: [2, 5], prefix: '', paused: false, forum: false, username: '', avatar_url: '', role_id: '', colour: '#ff4500', text: 'full' as const, image: true, author: true };

describe('notification endpoints', () => {
  it('offers every event the API knows', () => {
    expect(EVENTS.map((e) => e.key)).toEqual(['sign_up', 'link_request', 'blitz_report', 'blitz_flag', 'reddit_failing']);
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
    ['given a display name', { ...SAVED, username: 'TTA Reddit' }, true],
    ['given a picture', { ...SAVED, avatar_url: 'https://example.com/a.png' }, true],
    ['given a role to ping', { ...SAVED, role_id: '100000000000000002' }, true],
    ['given only spaces as a display name', { ...SAVED, username: '  ' }, false],
    ['given a colour', { ...SAVED, colour: '#123abc' }, true],
    ['showing less text', { ...SAVED, text: 'short' as const }, true],
    ['hiding the image', { ...SAVED, image: false }, true],
    ['hiding the author', { ...SAVED, author: false }, true],
    ['taking the same feeds in another order', { ...SAVED, feeds: [5, 2] }, false],
    ['left without a name', { ...SAVED, name: '  ' }, false],
    ['left without a URL', { ...SAVED, url: '' }, false],
  ])('can be saved when %s: %s', (_, draft, want) => {
    expect(endpointChanged(SAVED, draft)).toBe(want);
  });

  it.each([
    ['adding an event', ['blitz_flag' as const], 'link_request' as const, true, ['link_request', 'blitz_flag']],
    ['adding one already there', ['blitz_flag' as const], 'blitz_flag' as const, true, ['blitz_flag']],
    ['taking an event out', ['link_request' as const, 'blitz_flag' as const], 'link_request' as const, false, ['blitz_flag']],
  ])('toggles events: %s', (_, events, event, on, want) => {
    expect(toggled(events, event, on)).toEqual(want);
  });

  const FEEDS = [
    { id: 2, subreddit: 'throughtheages' },
    { id: 5, subreddit: 'boardgames' },
  ];

  it.each([
    ['nothing', [], [], 'Nothing: no posts reach it'],
    ['every event', ['sign_up', 'link_request', 'blitz_report', 'blitz_flag', 'reddit_failing'] as const, [], 'every site event'],
    ['some events, in the tab\'s order', ['blitz_flag', 'sign_up'] as const, [], 'New players signing up, Blitz reports flagged'],
    ['a feed', [], [5], 'r/boardgames'],
    ['events and feeds', ['link_request'] as const, [2, 5], 'Requests to be linked; r/throughtheages; r/boardgames'],
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
