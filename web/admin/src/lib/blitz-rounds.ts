/** The rounds card's decisions: when a round can be paired, which round may still change, who is unpaired, and the matches a change by hand produces. */
import type { components } from '@ttac/shared/api';

type Schemas = components['schemas'];
export type AdminBlitzRound = Schemas['AdminBlitzRound'];
export type AdminBlitzMatch = Schemas['AdminBlitzMatch'];
export type AdminBlitzEntrant = Schemas['AdminBlitzEntrant'];
/** MatchChange is a match as the API takes it back: entrant ids, b left out for the bye. */
export type MatchChange = { a: number; b?: number };

/** canPair says whether the next round can be paired: at least two active entrants and no draft open. */
export function canPair(entrants: readonly AdminBlitzEntrant[], rounds: readonly AdminBlitzRound[]): boolean {
  return entrants.filter((e) => e.status === 'active').length >= 2 && rounds.every((r) => r.published_at !== null);
}

/** newestFirst orders rounds by number, the latest first. */
export function newestFirst(rounds: readonly AdminBlitzRound[]): AdminBlitzRound[] {
  return [...rounds].sort((a, b) => b.number - a.number);
}

/** editableRound is the id of the round whose matches may still change: the latest one, or null with none. */
export function editableRound(rounds: readonly AdminBlitzRound[]): number | null {
  return newestFirst(rounds)[0]?.id ?? null;
}

/** unpaired lists the active entrants a round leaves out, in sign-up order. */
export function unpaired(entrants: readonly AdminBlitzEntrant[], round: AdminBlitzRound): AdminBlitzEntrant[] {
  const paired = new Set<number>();
  for (const m of round.matches) {
    paired.add(m.a);
    if (m.b !== null) paired.add(m.b);
  }
  return entrants.filter((e) => e.status === 'active' && !paired.has(e.id));
}

function changes(matches: readonly AdminBlitzMatch[]): MatchChange[] {
  return matches.map((m) => (m.b === null ? { a: m.a } : { a: m.a, b: m.b }));
}

/** withoutMatch is the round's matches without the one given, as the API takes them. */
export function withoutMatch(matches: readonly AdminBlitzMatch[], id: number): MatchChange[] {
  return changes(matches.filter((m) => m.id !== id));
}

/** withPair is the round's matches plus a new pair. */
export function withPair(matches: readonly AdminBlitzMatch[], a: number, b: number): MatchChange[] {
  return [...changes(matches), { a, b }];
}

/** withBye is the round's matches with the bye given to one entrant, taken from whoever had it. */
export function withBye(matches: readonly AdminBlitzMatch[], a: number): MatchChange[] {
  return [...changes(matches.filter((m) => m.b !== null)), { a }];
}

/** zoneGapLabel says how far apart two players' clocks are. */
export function zoneGapLabel(minutes: number): string {
  if (minutes === 0) return 'same hour';
  const h = Math.floor(minutes / 60);
  const m = minutes % 60;
  return m === 0 ? `${h} h apart` : `${h} h ${m} apart`;
}
