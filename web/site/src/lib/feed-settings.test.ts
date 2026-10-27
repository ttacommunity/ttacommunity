import { describe, expect, it } from 'vitest';
import {
  DEFAULT_FEED_SETTINGS,
  PREMIER_TOURNAMENTS,
  loadFeedSettings,
  saveFeedSettings,
  sanitizeFeedSettings,
  withPlayer,
  withoutPlayer,
  withoutTournament,
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
      players: ['ann', 'Bob'],
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

  it('allows multiple divisions of the same tournament while deduplicating exact matches', () => {
    const multi = sanitizeFeedSettings({
      tournaments: [
        { slug: 'royal_league', name: 'Royal League', division: 'Emperor' },
        { slug: 'royal_league', name: 'Royal League', division: 'King 1' },
        { slug: 'royal_league', name: 'Royal League', division: 'Emperor' }, // duplicate
      ],
      players: [],
    });
    expect(multi.tournaments).toEqual([
      { slug: 'royal_league', name: 'Royal League', division: 'Emperor' },
      { slug: 'royal_league', name: 'Royal League', division: 'King 1' },
    ]);
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

const league: FeedSettings = {
  tournaments: [
    { slug: 'royal_league', name: 'Royal League', division: 'Emperor' },
    { slug: 'royal_league', name: 'Royal League', division: 'King 1' },
    { slug: 'royal_league', name: 'Royal League', division: '' },
    { slug: 'intermezzo', name: 'Intermezzo Championship' },
  ],
  players: ['Ada', 'Bram'],
};

describe('withoutTournament', () => {
  it.each([
    ['one division of several', 'royal_league', 'King 1', ['royal_league Emperor', 'royal_league ', 'intermezzo ']],
    ['all divisions, written as empty', 'royal_league', '', ['royal_league Emperor', 'royal_league King 1', 'intermezzo ']],
    ['all divisions, written as none', 'intermezzo', undefined, ['royal_league Emperor', 'royal_league King 1', 'royal_league ']],
    ['a division not chosen', 'royal_league', 'King 2', ['royal_league Emperor', 'royal_league King 1', 'royal_league ', 'intermezzo ']],
    ['a tournament not chosen', 'international', 'Grandmaster', ['royal_league Emperor', 'royal_league King 1', 'royal_league ', 'intermezzo ']],
  ])('removes %s', (_, slug, division, want) => {
    const got = withoutTournament(league, slug, division);
    expect(got.tournaments.map((t) => `${t.slug} ${t.division ?? ''}`)).toEqual(want);
    expect(got.players).toEqual(league.players);
  });
});

describe('withPlayer and withoutPlayer', () => {
  it.each([
    ['adds a new name', 'Cleo', ['Ada', 'Bram', 'Cleo']],
    ['trims it', '  Cleo ', ['Ada', 'Bram', 'Cleo']],
    ['keeps a name already there in another case', 'ada', ['Ada', 'Bram']],
    ['ignores an empty name', '   ', ['Ada', 'Bram']],
  ])('withPlayer %s', (_, name, want) => {
    expect(withPlayer(league, name).players).toEqual(want);
  });

  it.each([
    ['removes a name', 'Bram', ['Ada']],
    ['ignoring case', 'ADA', ['Bram']],
    ['leaves the rest for a name not there', 'Cleo', ['Ada', 'Bram']],
  ])('withoutPlayer %s', (_, name, want) => {
    const got = withoutPlayer(league, name);
    expect(got.players).toEqual(want);
    expect(got.tournaments).toEqual(league.tournaments);
  });
});
