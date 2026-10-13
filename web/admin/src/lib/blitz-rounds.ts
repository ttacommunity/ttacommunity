/** The rounds card's decisions: when a round can be paired, which round may still change, who is unpaired, and the matches a change by hand produces. */
import type { components } from '@ttac/shared/api';

type Schemas = components['schemas'];
export type AdminBlitzRound = Schemas['AdminBlitzRound'];
export type AdminBlitzMatch = Schemas['AdminBlitzMatch'];
export type AdminBlitzEntrant = Schemas['AdminBlitzEntrant'];
/** MatchChange is a match as the API takes it back: entrant ids, b left out for the bye. */
export type MatchChange = { a: number; b?: number };

/** canPair says whether the next round can be paired: at least two active entrants, no draft open, and every match so far decided (byes aside). */
export function canPair(entrants: readonly AdminBlitzEntrant[], rounds: readonly AdminBlitzRound[]): boolean {
  return entrants.filter((e) => e.status === 'active').length >= 2 && rounds.every((r) => r.published_at !== null) && undecided(rounds, String).length === 0;
}

/** undecided names the matches still without a result, byes aside, oldest round first, as "A v B". */
export function undecided(rounds: readonly AdminBlitzRound[], nameOf: (id: number) => string): string[] {
  return [...rounds]
    .sort((a, b) => a.number - b.number)
    .flatMap((r) => r.matches.filter((m) => m.b !== null && m.result === null).map((m) => `${nameOf(m.a)} v ${nameOf(m.b as number)}`));
}

/** newestFirst orders rounds by number, the latest first. */
export function newestFirst(rounds: readonly AdminBlitzRound[]): AdminBlitzRound[] {
  return [...rounds].sort((a, b) => b.number - a.number);
}

/** editableRound is the id of the round whose matches may still change: the latest one, or null with none. */
export function editableRound(rounds: readonly AdminBlitzRound[]): number | null {
  return newestFirst(rounds)[0]?.id ?? null;
}

/** unsavedMatch is a match as proposed or made by hand and not yet saved: no id, no gaps, no result. */
export function unsavedMatch(a: number, b: number | null): AdminBlitzMatch {
  return { id: 0, a, b, zone_gap_minutes: 0, rating_gap: null, repeat: false, result: null, game: null, ruling: '', decided_at: null };
}

/** pairKey names a pairing by its two sides, the bye's second as "bye". */
function pairKey(m: AdminBlitzMatch): string {
  return `${m.a}-${m.b ?? 'bye'}`;
}

/** samePairs says whether two sets of matches pair the same players the same way, in any order. */
export function samePairs(saved: readonly AdminBlitzMatch[], pending: readonly AdminBlitzMatch[]): boolean {
  const a = saved.map(pairKey).sort();
  const b = pending.map(pairKey).sort();
  return a.length === b.length && a.every((k, i) => k === b[i]);
}

/** canDeleteRound says whether a round can still be deleted: nothing in it has a result (it must also be the latest, which editableRound ensures). */
export function canDeleteRound(round: AdminBlitzRound): boolean {
  return round.matches.every((m) => m.result === null);
}

/** decidedOnly keeps the matches that have a result: what is left of a round once everyone else is unpaired, to pair it by hand. */
export function decidedOnly(matches: readonly AdminBlitzMatch[]): AdminBlitzMatch[] {
  return matches.filter((m) => m.result !== null);
}

/** inARound says whether any round, draft or published, names the entrant: such an entrant cannot be removed, only dropped. */
export function inARound(id: number, rounds: readonly AdminBlitzRound[]): boolean {
  return rounds.some((r) => r.matches.some((m) => m.a === id || m.b === id));
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

/** asChanges turns matches into what the API takes back: entrant ids, b left out for the bye. */
export function asChanges(matches: readonly AdminBlitzMatch[]): MatchChange[] {
  return matches.map((m) => (m.b === null ? { a: m.a } : { a: m.a, b: m.b }));
}

const changes = asChanges;

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
