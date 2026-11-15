/** The rating tracks, named and described once for both sites. */
import type { components } from '../api/schema';

/** Track is a rating track: the games a run rates. */
export type Track = components['schemas']['RankingTrack'];

/** TRACKS are the rating tracks in the order the sites offer them, the default first; short is for a narrow column. */
export const TRACKS: readonly { value: Track; name: string; short: string; desc: string }[] = [
  { value: 'community', name: 'Standard', short: 'Std', desc: 'Classical asynchronous community tournaments' },
  { value: 'superblitz', name: 'SuperBlitz', short: 'SB', desc: 'Exclusive SuperBlitz tournament ratings' },
  { value: 'community_superblitz', name: 'Standard + SuperBlitz', short: 'Std + SB', desc: 'Standard tournaments with SuperBlitz included' },
  { value: 'pulse', name: 'Pulse', short: 'Pulse', desc: 'TTA Pulse real-time blitz queue ratings' },
];

/** trackName names a track's slug, or returns the slug when it names none. */
export function trackName(slug: string): string {
  return TRACKS.find((t) => t.value === slug)?.name ?? slug;
}

/** trackShort is a track's short name, or the slug when it names none. */
export function trackShort(slug: string): string {
  return TRACKS.find((t) => t.value === slug)?.short ?? slug;
}
