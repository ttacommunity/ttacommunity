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

/** challengeHref is the address of a challenge's page. */
export function challengeHref(id: string): string {
  return `/challenges/${id}/`;
}

/** challengePlayerHref is the address of a player's challenges page. */
export function challengePlayerHref(name: string): string {
  return `/challenges/player/?${new URLSearchParams({ name })}`;
}

/** compareHref is the address comparing player a with b, or with someone still to choose. */
export function compareHref(a: string, b?: string): string {
  return `/challenges/compare/?${new URLSearchParams(b === undefined ? { a } : { a, b })}`;
}

/** Measure is how players are compared: their rating (each best as a % of the record, averaged), or their total domination. */
export type Measure = 'rating' | 'total';

interface Figures {
  total: number;
  totals: Record<SetId, number>;
  rating: number;
  ratings: Record<SetId, number>;
}

/** measureOf is a player's value in the measure, overall or for one set. */
export function measureOf(p: Figures, measure: Measure, set?: SetId): number {
  if (measure === 'rating') return set === undefined ? p.rating : p.ratings[set];
  return set === undefined ? p.total : p.totals[set];
}

/** formatMeasure writes a rating to a tenth of a percent, a total as it is. */
export function formatMeasure(value: number, measure: Measure): string {
  return measure === 'rating' ? `${value.toFixed(1)}%` : String(value);
}

/** scopeLeaders are a scope's best players in the measure. */
export function scopeLeaders<P>(scope: { by_total: P[]; by_rating: P[] }, measure: Measure): P[] {
  return measure === 'rating' ? scope.by_rating : scope.by_total;
}

interface PlayerBest {
  challenge: string;
  domination: number;
  rank: number;
  share: number;
  record: number;
}

/** playNext picks, from bests not holding the record, the show closest to their record and the show lowest, in the measure. */
export function playNext<B extends PlayerBest>(bests: readonly B[], measure: Measure, show: number): { closest: B[]; lowest: B[] } {
  const open = bests.filter((b) => b.rank > 1);
  const closest = [...open].sort((a, b) => (measure === 'rating' ? b.share - a.share : a.record - a.domination - (b.record - b.domination)));
  const lowest = [...open].sort((a, b) => (measure === 'rating' ? a.share - b.share : a.domination - b.domination));
  return { closest: closest.slice(0, show), lowest: lowest.slice(0, show) };
}

/** compareBests counts, on the challenges both players played, a's wins, b's wins and the ties. */
export function compareBests(
  a: readonly { challenge: string; domination: number }[],
  b: readonly { challenge: string; domination: number }[],
): { winsA: number; winsB: number; ties: number; both: number } {
  const theirs = new Map(b.map((x) => [x.challenge, x.domination]));
  const got = { winsA: 0, winsB: 0, ties: 0, both: 0 };
  for (const x of a) {
    const other = theirs.get(x.challenge);
    if (other === undefined) continue;
    got.both++;
    if (x.domination > other) got.winsA++;
    else if (other > x.domination) got.winsB++;
    else got.ties++;
  }
  return got;
}

/** signed writes a difference with its sign, ±0 for none. */
export function signed(value: number): string {
  if (value === 0) return '±0';
  return value > 0 ? `+${value}` : String(value);
}

/** niceTicks are round values from at most min (and 0) to at least max, about count steps apart. */
export function niceTicks(min: number, max: number, count = 4): number[] {
  const lo = Math.min(0, min);
  const hi = Math.max(0, max);
  const raw = (hi - lo) / count;
  const mag = 10 ** Math.floor(Math.log10(raw || 1));
  const step = [1, 2, 2.5, 5, 10].map((m) => m * mag).find((s) => s >= raw) ?? raw;
  const ticks = [Math.floor(lo / step) * step];
  while (ticks[ticks.length - 1] < hi) ticks.push(ticks[ticks.length - 1] + step);
  return ticks;
}
