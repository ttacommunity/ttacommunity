import type { components } from '@ttac/shared/api';

export type MovieCreator = components['schemas']['MovieCreator'];
export type MovieVideo = components['schemas']['MovieVideo'];
export type MoviesFeed = components['schemas']['MoviesFeed'];

export type MovieSort = components['schemas']['MovieSort'];

/** A view of the Movies page: the creators or player chosen (every creator when neither is), the sort and the page. */
export interface MoviesView {
  creators: string[];
  player: string;
  sort: MovieSort;
  page: number;
}

/** The sorts the Movies page offers, in the order it shows them. */
export const movieSorts: readonly { value: MovieSort; label: string }[] = [
  { value: 'newest', label: 'Newest' },
  { value: 'oldest', label: 'Oldest' },
  { value: 'title', label: 'Title' },
  { value: 'creator', label: 'Creator' },
];

/** Reads a view from the page's address; an unknown sort or a bad page gives the default. */
export function moviesView(search: string): MoviesView {
  const q = new URLSearchParams(search);
  const sort = movieSorts.find((s) => s.value === q.get('sort'))?.value ?? 'newest';
  const page = Number(q.get('page') ?? '1');
  return {
    creators: q.getAll('creator'),
    player: q.get('player') ?? '',
    sort,
    page: Number.isInteger(page) && page >= 1 ? page : 1,
  };
}

/** Writes a view as both the page's address and the API's query, leaving out the defaults. */
export function moviesParams(view: MoviesView): URLSearchParams {
  const q = new URLSearchParams();
  if (view.player !== '') q.set('player', view.player);
  for (const c of view.creators) q.append('creator', c);
  if (view.sort !== 'newest') q.set('sort', view.sort);
  if (view.page > 1) q.set('page', String(view.page));
  return q;
}

/** The channels a view chooses: its creators, or every channel of its player in any case; none for every creator. */
export function chosenCreators(view: MoviesView, creators: readonly MovieCreator[]): Set<string> {
  const player = view.player.toLowerCase();
  return new Set(creators.filter((c) => view.creators.includes(c.channel_id) || (player !== '' && c.player?.toLowerCase() === player)).map((c) => c.channel_id));
}

/** Turns one creator on or off, a player's choice becoming their channels first, back on the first page. */
export function withCreator(view: MoviesView, creators: readonly MovieCreator[], channelId: string): MoviesView {
  const chosen = chosenCreators(view, creators);
  if (chosen.has(channelId)) chosen.delete(channelId);
  else chosen.add(channelId);
  return { ...view, creators: [...chosen], player: '', page: 1 };
}

/** How many videos the page shows from every creator. */
export function everyVideo(creators: readonly MovieCreator[]): number {
  return creators.reduce((n, c) => n + c.videos, 0);
}

/** Appends the next page's videos, leaving out any already shown: a new video moves each later page along by one. */
export function addPage(shown: readonly MovieVideo[], next: readonly MovieVideo[]): MovieVideo[] {
  const ids = new Set(shown.map((v) => v.video_id));
  return [...shown, ...next.filter((v) => !ids.has(v.video_id))];
}

/** Returns the standard YouTube watch URL for a video ID. */
export function youtubeWatchUrl(videoId: string): string {
  return `https://www.youtube.com/watch?v=${encodeURIComponent(videoId)}`;
}

/** Returns the privacy-enhanced YouTube embed URL for a video ID. */
export function youtubeEmbedUrl(videoId: string): string {
  return `https://www.youtube-nocookie.com/embed/${encodeURIComponent(videoId)}?autoplay=1&rel=0`;
}

/** Formats an ISO UTC timestamp in the viewer's local date. */
export function formatMovieDate(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium' }).format(d);
}

/** The Movies page filtered to a player's channels. */
export function moviesHref(player: string): string {
  return `/movies/?${new URLSearchParams({ player })}`;
}
