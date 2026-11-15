import { describe, expect, it } from 'vitest';
import { creatorVideoCounts, filterVideosByCreators, formatMovieDate, youtubeEmbedUrl, youtubeWatchUrl, type MovieVideo } from './movies';

const sample: MovieVideo[] = [
  {
    video_id: 'v1',
    channel_id: 'UC1',
    creator_name: 'Alice Casts',
    creator_player: 'Alice',
    title: 'Royal League vs Invented Rival',
    published_at: '2026-11-14T12:00:00Z',
    thumbnail_url: 'https://i.ytimg.com/vi/v1/hqdefault.jpg',
  },
  {
    video_id: 'v2',
    channel_id: 'UC2',
    creator_name: 'Bob Plays',
    creator_player: null,
    title: 'Through the Ages Strategy',
    published_at: '2026-11-13T12:00:00Z',
    thumbnail_url: 'https://i.ytimg.com/vi/v2/hqdefault.jpg',
  },
];

describe('filterVideosByCreators', () => {
  it('returns all videos when no creator filter is selected', () => {
    expect(filterVideosByCreators(sample, new Set())).toEqual(sample);
  });

  it('returns only videos matching the selected creator channel IDs', () => {
    const filtered = filterVideosByCreators(sample, new Set(['UC2']));
    expect(filtered.map((v) => v.video_id)).toEqual(['v2']);
  });
});

describe('creatorVideoCounts', () => {
  it('counts videos per channel ID', () => {
    const counts = creatorVideoCounts(sample);
    expect(counts.get('UC1')).toBe(1);
    expect(counts.get('UC2')).toBe(1);
  });
});

describe('youtube URLs', () => {
  it('builds watch and nocookie embed URLs', () => {
    expect(youtubeWatchUrl('vid00000001')).toBe('https://www.youtube.com/watch?v=vid00000001');
    expect(youtubeEmbedUrl('vid00000001')).toBe('https://www.youtube-nocookie.com/embed/vid00000001?autoplay=1&rel=0');
  });
});

describe('formatMovieDate', () => {
  it('falls back to the raw string on invalid dates', () => {
    expect(formatMovieDate('not-a-date')).toBe('not-a-date');
  });
});
