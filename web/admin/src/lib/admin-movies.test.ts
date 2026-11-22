import { describe, expect, it } from 'vitest';
import { buildNewChannelBody, channelStatus, channelTone, moviesRunStarted, overrideChoice, overrideOf, NO_VIDEO_FILTERS, reasonKind, videoPage, videosMatching, videoStatus, videoTone, type AdminVideo, type AdminVideoChannel } from './admin-movies';

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
  videos_found: 0,
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
      'an error in the run that also subscribed it',
      channel({ subscribed_at: '2026-11-15T03:00:01Z', lease_ends_at: '2026-11-20T03:00:01Z', last_error: 'filling in its videos: HTTP 404', last_error_at: '2026-11-15T03:00:00Z' }),
      { kind: 'failing', text: 'Failing since 2026-11-15: filling in its videos: HTTP 404' },
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

describe('moviesRunStarted', () => {
  it.each([
    ['every creator', null, 'Started the Movies job for every creator. Each row shows how it went when the job ends, in a minute or two.'],
    ['one creator', 'Invented Casts', 'Started the Movies job for Invented Casts. Its row shows how it went when the job ends, in a minute or two.'],
  ] as const)('%s', (_, name, want) => {
    expect(moviesRunStarted(name)).toBe(want);
  });
});

describe('videosMatching', () => {
  const videos = [
    video({ video_id: 'vid00000001', channel_id: 'UC1', title: 'Invented Open final', published_at: '2026-11-14T10:00:00Z', tta_reason: 'names the tournament Invented Open' }),
    video({ video_id: 'vid00000002', channel_id: 'UC2', title: 'Chess opening', published_at: '2025-03-01T10:00:00Z', is_tta: false, tta_reason: 'no mention of TTA', shown: false }),
    video({ video_id: 'vid00000003', channel_id: 'UC2', title: 'A tutorial', published_at: '2026-10-20T10:00:00Z', tta_override: false, shown: false }),
    video({ video_id: 'vid00000004', channel_id: 'UC3', title: 'Everything', published_at: '2026-08-20T10:00:00Z', tta_reason: "the channel's every video counts" }),
  ];
  const dayOf = (iso: string) => iso.slice(0, 10);
  it.each([
    ['no filters', {}, ['vid00000001', 'vid00000002', 'vid00000003', 'vid00000004']],
    ['by title, any case', { title: 'OPEN' }, ['vid00000001', 'vid00000002']],
    ['by creators', { creators: ['UC2', 'UC3'] }, ['vid00000002', 'vid00000003', 'vid00000004']],
    ['the last 7 days, today included', { date: '7d' }, ['vid00000001']],
    ['the last 30 days', { date: '30d' }, ['vid00000001', 'vid00000003']],
    ['the last 3 months, by the calendar', { date: '3m' }, ['vid00000001', 'vid00000003', 'vid00000004']],
    ['the last 12 months', { date: '12m' }, ['vid00000001', 'vid00000003', 'vid00000004']],
    ['on or after a day', { date: 'custom', from: '2026-10-20' }, ['vid00000001', 'vid00000003']],
    ['on or before a day', { date: 'custom', to: '2026-08-20' }, ['vid00000002', 'vid00000004']],
    ['between two days', { date: 'custom', from: '2026-08-01', to: '2026-10-31' }, ['vid00000003', 'vid00000004']],
    ['by status', { statuses: ['hidden'] }, ['vid00000002', 'vid00000003']],
    ['by reason', { reasons: ['tournament', 'every'] }, ['vid00000001', 'vid00000004']],
    ['by who decides', { shownBy: ['hide', 'show'] }, ['vid00000003']],
    ['every filter at once', { creators: ['UC2'], statuses: ['hidden'], shownBy: ['verdict'] }, ['vid00000002']],
  ] as const)('%s', (_, filters, want) => {
    expect(videosMatching(videos, { ...NO_VIDEO_FILTERS, ...filters }, dayOf, '2026-11-16').map((v) => v.video_id)).toEqual(want);
  });
});

describe('reasonKind', () => {
  it.each([
    ["the channel's every video counts", 'every'],
    ['mentions Through the Ages', 'mentions'],
    ['mentions TTA', 'mentions'],
    ['names the tournament Invented Open', 'tournament'],
    ['no mention of TTA', 'none'],
    ['test', 'other'],
  ] as const)('%s', (reason, want) => {
    expect(reasonKind(reason)).toBe(want);
  });
});

describe('videoPage', () => {
  const ids = Array.from({ length: 120 }, (_, i) => video({ video_id: `vid${String(i).padStart(8, '0')}` }));
  it.each([
    ['the first page', 1, 1, 3, 'vid00000000', 50],
    ['the last, part-full', 3, 3, 3, 'vid00000100', 20],
    ['past the end, after a filter shrank the list', 9, 3, 3, 'vid00000100', 20],
    ['before the start', 0, 1, 3, 'vid00000000', 50],
  ] as const)('%s', (_, asked, page, pages, first, rows) => {
    const got = videoPage(ids, asked);
    expect([got.page, got.pages, got.rows[0]?.video_id, got.rows.length]).toEqual([page, pages, first, rows]);
  });
  it('one empty page for no videos', () => {
    expect(videoPage([], 1)).toEqual({ page: 1, pages: 1, rows: [] });
  });
});

describe('channelTone', () => {
  it.each([
    { name: 'a working channel is good', kind: 'ok' as const, want: 'good' },
    { name: 'a channel waiting for its first fetch is muted', kind: 'waiting' as const, want: 'muted' },
    { name: 'a failing channel is bad', kind: 'failing' as const, want: 'bad' },
  ])('$name', ({ kind, want }) => {
    expect(channelTone(kind)).toBe(want);
  });
});

describe('videoTone', () => {
  it.each([
    { name: 'a shown video is good', shown: true, want: 'good' },
    { name: 'a hidden video is muted', shown: false, want: 'muted' },
  ])('$name', ({ shown, want }) => {
    expect(videoTone(shown)).toBe(want);
  });
});
