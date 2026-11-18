import { describe, expect, it } from 'vitest';
import { feedGroups, feedRows, optionsFor, twitchStatus, withOptions, words } from './feeds';

const when = (iso: string) => `at ${iso}`;
const reddit = (id: number, subreddit: string, paused = false) => ({
  id, subreddit, paused, kinds: ['text' as const, 'image' as const, 'gallery' as const, 'video' as const, 'link' as const],
  checked_at: '2026-11-18T08:00:00Z',
});
const twitch = (id: number, login: string, statuses: string[], paused = false) => ({
  id, login, display_name: login, user_id: '100000001', paused, created_at: '2026-11-18T08:00:00Z',
  subscriptions: statuses.map((status, i) => ({ type: i === 0 ? 'stream.online' : 'stream.offline', status })),
});

describe('feeds', () => {
  it('lists Reddit feeds then Twitch streamers, each by name', () => {
    const rows = feedRows([reddit(2, 'throughtheages'), reddit(1, 'boardgames')], [twitch(3, 'some_streamer', ['enabled', 'enabled'])], when);
    expect(rows.map((r) => `${r.kind} ${r.name} ${r.href}`)).toEqual([
      'reddit r/boardgames https://www.reddit.com/r/boardgames/new/',
      'reddit r/throughtheages https://www.reddit.com/r/throughtheages/new/',
      'twitch some_streamer https://www.twitch.tv/some_streamer',
    ]);
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
    const groups = feedGroups([reddit(2, 'throughtheages', true)], [twitch(3, 'some_streamer', [])]);
    expect(groups.map((g) => `${g.label}: ${g.feeds.map((f) => `${f.name}${f.paused ? ' (paused)' : ''}`).join(', ')}`)).toEqual([
      'Reddit: r/throughtheages (paused)',
      'Twitch: some_streamer',
    ]);
    expect(feedGroups([], [twitch(3, 'some_streamer', [])]).map((g) => g.kind)).toEqual(['twitch']);
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
    expect(optionsFor([], 3)).toEqual({ feed: 3, categories: [], keywords: [], was_live: false });
    const set = withOptions([], 3, { keywords: ['tta'], was_live: true });
    expect(set).toEqual([{ feed: 3, categories: [], keywords: ['tta'], was_live: true }]);
    expect(withOptions(set, 3, { keywords: [], was_live: false })).toEqual([]);
  });
});
