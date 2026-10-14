import { describe, expect, it } from 'vitest';
import {
  DEFAULT_FEED_SETTINGS,
  PREMIER_TOURNAMENTS,
  loadFeedSettings,
  saveFeedSettings,
  sanitizeFeedSettings,
  type FeedSettings,
} from './feed-settings';

describe('feed-settings', () => {
  it('provides the three premier tournaments as defaults', () => {
    expect(DEFAULT_FEED_SETTINGS.tournaments).toEqual(PREMIER_TOURNAMENTS);
    expect(DEFAULT_FEED_SETTINGS.tournaments.map((t) => t.slug)).toEqual([
      'international',
      'intermezzo',
      'royal_league',
    ]);
    expect(DEFAULT_FEED_SETTINGS.tournaments.map((t) => t.division)).toEqual([
      'Grandmaster',
      'Grandmaster',
      'Emperor',
    ]);
    expect(DEFAULT_FEED_SETTINGS.players).toEqual([]);
  });

  it('saves and loads valid settings to/from storage', () => {
    const store = new Map<string, string>();
    const mockStorage = {
      getItem: (k: string) => store.get(k) ?? null,
      setItem: (k: string, v: string) => store.set(k, v),
      removeItem: (k: string) => store.delete(k),
      clear: () => store.clear(),
      length: 0,
      key: () => null,
    } as unknown as Storage;

    expect(loadFeedSettings(mockStorage)).toEqual(DEFAULT_FEED_SETTINGS);

    const custom: FeedSettings = {
      tournaments: [
        { slug: 'royal_league', name: 'Royal League', division: 'King 1' },
      ],
      players: ['pajada', 'BlixLT'],
    };

    saveFeedSettings(mockStorage, custom);
    expect(loadFeedSettings(mockStorage)).toEqual(custom);
  });

  it('sanitizes corrupted or unexpected storage data', () => {
    expect(sanitizeFeedSettings(null)).toEqual(DEFAULT_FEED_SETTINGS);
    expect(sanitizeFeedSettings('invalid json string')).toEqual(DEFAULT_FEED_SETTINGS);
    expect(sanitizeFeedSettings({})).toEqual(DEFAULT_FEED_SETTINGS);
    expect(
      sanitizeFeedSettings({
        tournaments: [{ slug: 'custom_tourney', name: 'Custom Tourney' }],
        players: ['Player1', 123, null],
      }),
    ).toEqual({
      tournaments: [{ slug: 'custom_tourney', name: 'Custom Tourney', division: '' }],
      players: ['Player1'],
    });
  });

  it('gracefully handles storage errors (e.g. quota or blocked access)', () => {
    const failingStorage = {
      getItem: () => {
        throw new Error('Blocked');
      },
      setItem: () => {
        throw new Error('Quota exceeded');
      },
      removeItem: () => {},
      clear: () => {},
      length: 0,
      key: () => null,
    } as unknown as Storage;

    expect(loadFeedSettings(failingStorage)).toEqual(DEFAULT_FEED_SETTINGS);
    expect(() => saveFeedSettings(failingStorage, DEFAULT_FEED_SETTINGS)).not.toThrow();
  });
});
