import type { components } from '@ttac/shared/api';

export type MovieCreator = components['schemas']['MovieCreator'];
export type MovieVideo = components['schemas']['MovieVideo'];
export type MoviesFeed = components['schemas']['MoviesFeed'];

/** Filters videos to those from the selected channel IDs; when none are selected, returns all videos. */
export function filterVideosByCreators(videos: readonly MovieVideo[], selectedChannelIds: ReadonlySet<string>): MovieVideo[] {
  if (selectedChannelIds.size === 0) return [...videos];
  return videos.filter((v) => selectedChannelIds.has(v.channel_id));
}

/** Counts how many videos each creator channel has in the feed. */
export function creatorVideoCounts(videos: readonly MovieVideo[]): Map<string, number> {
  const counts = new Map<string, number>();
  for (const v of videos) counts.set(v.channel_id, (counts.get(v.channel_id) ?? 0) + 1);
  return counts;
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

/** The channels of the player the address's ?player= names, in any case; none when it names nobody. */
export function creatorsOfPlayer(creators: readonly MovieCreator[], search: string): Set<string> {
  const player = (new URLSearchParams(search).get('player') ?? '').trim().toLowerCase();
  if (player === '') return new Set();
  return new Set(creators.filter((c) => c.player?.toLowerCase() === player).map((c) => c.channel_id));
}
