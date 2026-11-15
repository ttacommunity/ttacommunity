import { describe, expect, it } from 'vitest';
import { buildNewChannelBody, channelStatus, overrideChoice, overrideOf, videoStatus, type AdminVideo, type AdminVideoChannel } from './admin-movies';

describe('buildNewChannelBody', () => {
  it('requires channel and name', () => {
    expect(buildNewChannelBody('  ', 'Alice', '', false)).toEqual({
      ok: false,
      message: 'Enter a YouTube channel URL, @handle, or UC... ID.',
    });
    expect(buildNewChannelBody('https://youtube.com/@alice', '  ', '', false)).toEqual({
      ok: false,
      message: 'Enter a creator display name.',
    });
  });

  it('builds a trimmed payload with optional player', () => {
    expect(buildNewChannelBody(' @alice ', ' Alice Casts ', ' Alice ', true)).toEqual({
      ok: true,
      body: { channel: '@alice', name: 'Alice Casts', player: 'Alice', include_all: true },
    });
    expect(buildNewChannelBody('UC123', 'Bob Plays', '  ', false)).toEqual({
      ok: true,
      body: { channel: 'UC123', name: 'Bob Plays', include_all: false },
    });
  });
});

const video = (over: Partial<AdminVideo>): AdminVideo => ({
  video_id: 'vid00000001',
  channel_id: 'UC0000000000000000000001',
  creator_name: 'Invented Caster',
  title: 'Game 5',
  published_at: '2026-11-15T08:00:00Z',
  thumbnail_url: '',
  is_tta: true,
  tta_reason: 'mentions Through the Ages',
  tta_override: null,
  shown: true,
  ...over,
});

describe('overrides', () => {
  it.each([
    [null, 'verdict'],
    [true, 'show'],
    [false, 'hide'],
  ] as const)('override %s is the choice %s, and back', (override, choice) => {
    expect(overrideChoice(video({ tta_override: override }))).toBe(choice);
    expect(overrideOf(choice)).toBe(override);
  });
});

describe('videoStatus', () => {
  it.each([
    ['a TTA video', video({}), { shown: true, why: 'mentions Through the Ages' }],
    ['a rejected video', video({ is_tta: false, tta_reason: 'no mention of TTA', shown: false }), { shown: false, why: 'no mention of TTA' }],
    ['a rejected video an admin shows', video({ is_tta: false, tta_reason: 'no mention of TTA', tta_override: true, shown: true }), { shown: true, why: 'by an admin (no mention of TTA)' }],
    ['a TTA video an admin keeps off', video({ tta_override: false, shown: false }), { shown: false, why: 'by an admin (mentions Through the Ages)' }],
  ])('%s', (_name, v, want) => {
    expect(videoStatus(v)).toEqual(want);
  });
});

const channel = (over: Partial<AdminVideoChannel>): AdminVideoChannel => ({
  channel_id: 'UC0000000000000000000001',
  name: 'Invented Caster',
  include_all: false,
  created_at: '2026-11-01T00:00:00Z',
  video_count: 0,
  subscribed_at: null,
  lease_ends_at: null,
  last_push_at: null,
  last_error: null,
  last_error_at: null,
  ...over,
});

describe('channelStatus', () => {
  const now = new Date('2026-11-15T09:00:00Z');
  const day = (iso: string) => iso.slice(0, 10);
  it.each([
    ['not yet subscribed', channel({}), { kind: 'waiting', text: 'Not subscribed yet' }],
    [
      'subscribed',
      channel({ subscribed_at: '2026-11-14T09:00:00Z', lease_ends_at: '2026-11-19T09:00:00Z' }),
      { kind: 'ok', text: 'Subscribed until 2026-11-19' },
    ],
    [
      'subscribed, with a push',
      channel({ subscribed_at: '2026-11-14T09:00:00Z', lease_ends_at: '2026-11-19T09:00:00Z', last_push_at: '2026-11-15T07:00:00Z' }),
      { kind: 'ok', text: 'Subscribed until 2026-11-19; last video 2026-11-15' },
    ],
    [
      'a lease run out',
      channel({ subscribed_at: '2026-11-01T09:00:00Z', lease_ends_at: '2026-11-06T09:00:00Z' }),
      { kind: 'failing', text: 'Subscription ended 2026-11-06' },
    ],
    [
      'an error since the last subscription',
      channel({ subscribed_at: '2026-11-14T09:00:00Z', lease_ends_at: '2026-11-19T09:00:00Z', last_error: 'the hub answered 500', last_error_at: '2026-11-15T03:00:00Z' }),
      { kind: 'failing', text: 'Failing since 2026-11-15: the hub answered 500' },
    ],
    [
      'an error before the last subscription',
      channel({ subscribed_at: '2026-11-14T09:00:00Z', lease_ends_at: '2026-11-19T09:00:00Z', last_error: 'the hub answered 500', last_error_at: '2026-11-13T03:00:00Z' }),
      { kind: 'ok', text: 'Subscribed until 2026-11-19' },
    ],
    [
      'an error and never subscribed',
      channel({ last_error: 'the hub answered 500', last_error_at: '2026-11-13T03:00:00Z' }),
      { kind: 'failing', text: 'Failing since 2026-11-13: the hub answered 500' },
    ],
  ])('%s', (_name, c, want) => {
    expect(channelStatus(c, now, day)).toEqual(want);
  });
});
