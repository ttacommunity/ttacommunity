import { describe, expect, it } from 'vitest';
import { feedStatus } from './reddit-feeds';

const when = (iso: string) => `[${iso}]`;
const FEED = { id: 1, subreddit: 'example', paused: false };

describe('reddit feeds', () => {
  it.each([
    ['new', FEED, 'Not read yet: its first read posts nothing'],
    ['read, no posts', { ...FEED, checked_at: '2026-11-17T01:00:00Z' }, 'Read [2026-11-17T01:00:00Z]'],
    ['read, with a post', { ...FEED, checked_at: '2026-11-17T01:00:00Z', last_post_at: '2026-11-14T10:00:00Z' }, 'Read [2026-11-17T01:00:00Z]; newest post [2026-11-14T10:00:00Z]'],
    ['failing', { ...FEED, checked_at: '2026-11-17T01:00:00Z', last_error: 'Reddit answered 429', last_error_at: '2026-11-17T02:00:00Z' }, 'Failed [2026-11-17T02:00:00Z]: Reddit answered 429'],
    ['paused', { ...FEED, paused: true, checked_at: '2026-11-17T01:00:00Z' }, 'Paused: not read'],
  ])('says how a feed is: %s', (_, feed, want) => {
    expect(feedStatus(feed, when)).toBe(want);
  });
});
