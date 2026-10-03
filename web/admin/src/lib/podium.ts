/** A season's podium as admin edits it: three name boxes, 1st to 3rd. */
import type { components } from '@ttac/shared/api';

export type PodiumEntry = components['schemas']['PodiumEntry'];
export type SeasonPodium = components['schemas']['SeasonPodium'];

/** podiumRequest builds the body that sets a season's podium from the three boxes, or null when all are empty. */
export function podiumRequest(season: string, names: string[], note: string): SeasonPodium | null {
  const podium = names.flatMap((n, i) => (n.trim() === '' ? [] : [{ place: i + 1, name: n.trim() }]));
  return podium.length === 0 ? null : { season, podium, note: note.trim() };
}

/** podiumNames lays a saved podium out as the three boxes' contents. */
export function podiumNames(podium: PodiumEntry[]): string[] {
  const names = ['', '', ''];
  for (const e of podium) names[e.place - 1] = e.name;
  return names;
}
