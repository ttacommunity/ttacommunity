import { describe, expect, it } from 'vitest';
import {
  addPage,
  chosenCreators,
  everyVideo,
  formatMovieDate,
  moviesHref,
  moviesParams,
  moviesView,
  withCreator,
  youtubeEmbedUrl,
  youtubeWatchUrl,
  type MovieCreator,
  type MoviesView,
  type MovieVideo,
} from './movies';

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

describe('moviesHref', () => {
  it.each([
    ['Ann', '/movies/?player=Ann'],
    ['Invented Caster', '/movies/?player=Invented+Caster'],
    ['a&b=c', '/movies/?player=a%26b%3Dc'],
  ])('links %s to the Movies page filtered to them', (name, want) => {
    expect(moviesHref(name)).toBe(want);
  });
});

const creators: MovieCreator[] = [
  { channel_id: 'UC1', name: 'Ann Casts', player: 'Ann', videos: 3 },
  { channel_id: 'UC2', name: 'Ann Again', player: 'Ann', videos: 4 },
  { channel_id: 'UC3', name: 'Bob Plays', player: 'Bob', videos: 5 },
  { channel_id: 'UC4', name: 'No Player', player: null, videos: 0 },
];

const start: MoviesView = { creators: [], player: '', sort: 'newest', page: 1 };

describe('moviesView', () => {
  it.each<[string, string, MoviesView]>([
    ['nothing chosen', '', start],
    ['a player', '?player=Ann', { ...start, player: 'Ann' }],
    ['several creators', '?creator=UC1&creator=UC3', { ...start, creators: ['UC1', 'UC3'] }],
    ['a sort and a page', '?sort=title&page=3', { ...start, sort: 'title', page: 3 }],
    ['an unknown sort', '?sort=views', start],
    ['a page that is not a number', '?page=two', start],
    ['page zero', '?page=0', start],
    ['a fractional page', '?page=1.5', start],
  ])('reads %s', (_name, search, want) => {
    expect(moviesView(search)).toEqual(want);
  });
});

describe('moviesParams', () => {
  it.each<[string, MoviesView, string]>([
    ['nothing for the first view', start, ''],
    ['a player', { ...start, player: 'Ann' }, 'player=Ann'],
    ['each creator', { ...start, creators: ['UC1', 'UC3'] }, 'creator=UC1&creator=UC3'],
    ['a sort and a page', { ...start, sort: 'oldest', page: 2 }, 'sort=oldest&page=2'],
  ])('writes %s', (_name, view, want) => {
    expect(moviesParams(view).toString()).toBe(want);
  });

  it('reads back what it writes', () => {
    const view: MoviesView = { creators: ['UC2'], player: '', sort: 'creator', page: 4 };
    expect(moviesView(`?${moviesParams(view)}`)).toEqual(view);
  });
});

describe('chosenCreators', () => {
  it.each<[string, MoviesView, string[]]>([
    ['none for every creator', start, []],
    ['the creators named', { ...start, creators: ['UC3'] }, ['UC3']],
    ["a player's channels", { ...start, player: 'Ann' }, ['UC1', 'UC2']],
    ['a player in any case', { ...start, player: 'bOB' }, ['UC3']],
  ])('chooses %s', (_name, view, want) => {
    expect([...chosenCreators(view, creators)]).toEqual(want);
  });
});

describe('withCreator', () => {
  it.each<[string, MoviesView, string, MoviesView]>([
    ['adds a creator', start, 'UC3', { ...start, creators: ['UC3'] }],
    ['removes a chosen creator', { ...start, creators: ['UC1', 'UC3'] }, 'UC1', { ...start, creators: ['UC3'] }],
    ["turns a player's choice into their channels", { ...start, player: 'Ann' }, 'UC3', { ...start, creators: ['UC1', 'UC2', 'UC3'] }],
    ['goes back to the first page', { ...start, page: 5 }, 'UC1', { ...start, creators: ['UC1'] }],
  ])('%s', (_name, view, channel, want) => {
    expect(withCreator(view, creators, channel)).toEqual(want);
  });
});

describe('everyVideo', () => {
  it('adds up every creator', () => {
    expect(everyVideo(creators)).toBe(12);
  });
});

describe('addPage', () => {
  const v = (id: string) => ({ video_id: id }) as MovieVideo;
  it.each<[string, string[], string[], string[]]>([
    ['appends the next page', ['a', 'b'], ['c', 'd'], ['a', 'b', 'c', 'd']],
    ['leaves out a video already shown, as when a new one moves the pages along', ['a', 'b'], ['b', 'c'], ['a', 'b', 'c']],
  ])('%s', (_name, shown, next, want) => {
    expect(addPage(shown.map(v), next.map(v)).map((x) => x.video_id)).toEqual(want);
  });
});
