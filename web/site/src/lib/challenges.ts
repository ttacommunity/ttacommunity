/** The Challenges section's decisions: how the challenges are grouped and ordered, and what their groups are called. */

export type SetId = 'original' | 'expansion' | 'mix';

const SETS: { id: SetId; label: string }[] = [
  { id: 'original', label: 'Original' },
  { id: 'expansion', label: 'Expansion' },
  { id: 'mix', label: 'Random Mix' },
];

const GROUP_LABELS: Record<string, string> = {
  classic: 'Classic',
  cheap: 'Low-Priced Lands',
  immortals: 'Immortals',
  epic: 'Epic Game',
  impacts: 'Deep Impact',
  time_travellers: 'Time Travelers',
  fate: 'Fate',
  gods: 'Gods',
  wonders: 'Wonders',
  kidnappers: 'Kidnappers',
  mix: 'Mix',
};

export interface ChallengeGroup<C> {
  id: string;
  label: string;
  challenges: C[];
}

export interface ChallengeSet<C> {
  id: SetId;
  label: string;
  groups: ChallengeGroup<C>[];
}

/** challengeSets splits the list into the game's sets, in the game's order, each into its groups where their first challenge is, the challenges as listed. A set with none is left out. */
export function challengeSets<C extends { set: SetId; group: string }>(list: readonly C[]): ChallengeSet<C>[] {
  return SETS.map(({ id, label }) => {
    const groups = new Map<string, C[]>();
    for (const c of list) {
      if (c.set !== id) continue;
      groups.set(c.group, [...(groups.get(c.group) ?? []), c]);
    }
    return { id, label, groups: [...groups].map(([group, challenges]) => ({ id: group, label: groupLabel(group), challenges })) };
  }).filter((s) => s.groups.length > 0);
}

/** groupLabel is the game's name for a group, or the id written as words for one it does not know. */
export function groupLabel(group: string): string {
  const words = group.replaceAll('_', ' ');
  return GROUP_LABELS[group] ?? words.charAt(0).toUpperCase() + words.slice(1);
}

/** challengePicture is the address of a challenge's picture. */
export function challengePicture(id: string): string {
  return `/img/challenges/${id}.jpg`;
}
