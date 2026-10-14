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

/** undecided names the matches still without a result, byes aside, oldest round first, as "A v B"; a knockout match with a result but no winner (a draw, a double loss) as "A v B (no winner yet)". */
export function undecided(rounds: readonly AdminBlitzRound[], nameOf: (id: number) => string): string[] {
  return [...rounds]
    .sort((a, b) => a.number - b.number)
    .flatMap((r) =>
      r.matches
        .filter((m) => m.b !== null && (m.result === null || (r.kind === 'knockout' && m.result !== 'a' && m.result !== 'b')))
        .map((m) => `${nameOf(m.a)} v ${nameOf(m.b as number)}${m.result === null ? '' : ' (no winner yet)'}`),
    );
}

/** NextRound is the round Pair makes next: its number, and whether it is a Swiss or a knockout round. */
export type NextRound = { number: number; kind: AdminBlitzRound['kind'] };

/** nextRound is the round Pair makes next: Swiss rounds until the format's are made, then the knockout's; null once there is a champion, or after the Swiss rounds when there is no knockout. */
export function nextRound(rounds: readonly AdminBlitzRound[], format: { swiss_rounds: number; knockout_size: number }, champion: string | null): NextRound | null {
  if (champion !== null) return null;
  const number = rounds.reduce((n, r) => Math.max(n, r.number), 0) + 1;
  if (rounds.filter((r) => r.kind === 'swiss').length < format.swiss_rounds) return { number, kind: 'swiss' };
  return format.knockout_size === 0 ? null : { number, kind: 'knockout' };
}

/** rulingChoices are the results a ruling may give: a knockout match needs a winner, so no draw or double loss there. */
export function rulingChoices(kind: AdminBlitzRound['kind']): NonNullable<AdminBlitzMatch['result']>[] {
  return kind === 'knockout' ? ['a', 'b'] : ['a', 'b', 'draw', 'double_loss'];
}

/** newestFirst orders rounds by number, the latest first. */
export function newestFirst(rounds: readonly AdminBlitzRound[]): AdminBlitzRound[] {
  return [...rounds].sort((a, b) => b.number - a.number);
}

/** openRound is the round still in play, the only one whose matches may change: the latest, while it is a draft or has a match to decide; null otherwise. */
export function openRound(rounds: readonly AdminBlitzRound[]): AdminBlitzRound | null {
  const latest = newestFirst(rounds)[0];
  if (latest === undefined) return null;
  return latest.published_at === null || undecided([latest], String).length > 0 ? latest : null;
}

/** PairAction is what Pair does: make the next round, pair the players the open round leaves out, or nothing. */
export type PairAction = { kind: 'next'; round: NextRound } | { kind: 'fill'; round: number } | { kind: 'none' };

type Edition = { entrants: readonly AdminBlitzEntrant[]; rounds: readonly AdminBlitzRound[]; format: { swiss_rounds: number; knockout_size: number }; champion: string | null };

/** pairAction is what Pair does for an edition, given the open round's matches as shown (null for as saved): fill in an open Swiss round's unpaired players; with no round open, make the next round. */
export function pairAction(ed: Edition, shown: readonly AdminBlitzMatch[] | null): PairAction {
  const open = openRound(ed.rounds);
  if (open !== null) {
    const left = unpaired(ed.entrants, { matches: shown ?? open.matches });
    return open.kind === 'swiss' && left.length > 0 ? { kind: 'fill', round: open.id } : { kind: 'none' };
  }
  const next = nextRound(ed.rounds, ed.format, ed.champion);
  return next !== null && canPair(ed.entrants, ed.rounds) ? { kind: 'next', round: next } : { kind: 'none' };
}

/** unsavedMatch is a match as proposed or made by hand and not yet saved: no id, no gaps, no result. */
export function unsavedMatch(a: number, b: number | null): AdminBlitzMatch {
  return { id: 0, a, b, slot: null, a_seed: null, b_seed: null, zone_gap_minutes: 0, rating_gap: null, repeat: false, result: null, game: null, ruling: '', decided_at: null };
}

/** pairKey names a pairing by its two players, whichever way round, the bye as "bye". */
function pairKey(m: AdminBlitzMatch): string {
  return m.b === null ? `${m.a}-bye` : `${Math.min(m.a, m.b)}-${Math.max(m.a, m.b)}`;
}

/** samePairs says whether two sets of matches pair the same players, in any order and either way round. */
export function samePairs(saved: readonly AdminBlitzMatch[], pending: readonly AdminBlitzMatch[]): boolean {
  const a = saved.map(pairKey).sort();
  const b = pending.map(pairKey).sort();
  return a.length === b.length && a.every((k, i) => k === b[i]);
}

/** shownAfter is what a round shows once its matches are changed: null (the saved ones) when the change pairs the same players, else the change with each saved pair kept as saved, so its results can still be recorded. */
export function shownAfter(saved: readonly AdminBlitzMatch[], changed: readonly AdminBlitzMatch[]): AdminBlitzMatch[] | null {
  if (samePairs(saved, changed)) return null;
  const byPair = new Map(saved.map((m) => [pairKey(m), m]));
  return changed.map((m) => byPair.get(pairKey(m)) ?? m);
}

/** Pending is a round's matches as changed in the page and not yet saved. */
export type Pending = { round: number; matches: AdminBlitzMatch[] };

/** reconcilePending keeps unsaved changes true to the edition as reloaded: none once their round is gone, without the matches of anyone no longer entered, and none when what is left pairs as saved. */
export function reconcilePending(pending: Pending | null, rounds: readonly AdminBlitzRound[], entrants: readonly { id: number }[]): Pending | null {
  if (pending === null) return null;
  const round = rounds.find((r) => r.id === pending.round);
  if (round === undefined) return null;
  const entered = new Set(entrants.map((e) => e.id));
  const shown = shownAfter(round.matches, pending.matches.filter((m) => entered.has(m.a) && (m.b === null || entered.has(m.b))));
  return shown === null ? null : { round: pending.round, matches: shown };
}

/** canDeleteRound says whether a round can still be deleted: nothing in it has a result (it must also be the latest, which editableRound ensures). */
export function canDeleteRound(round: AdminBlitzRound): boolean {
  return round.matches.every((m) => m.result === null);
}

/** decidedOnly keeps the matches that have a result: what is left of a round once everyone else is unpaired, to pair it by hand. */
export function decidedOnly(matches: readonly AdminBlitzMatch[]): AdminBlitzMatch[] {
  return matches.filter((m) => m.result !== null);
}

/** hasPlayed says whether an entrant has played: a match with a result, or a bye in a published round. */
export function hasPlayed(id: number, rounds: readonly AdminBlitzRound[]): boolean {
  return rounds.some((r) => r.matches.some((m) => (m.a === id || m.b === id) && (m.result !== null || (m.b === null && r.published_at !== null))));
}

/** EntrantButtons are the buttons an entrant's row shows: Confirm, Drop or Reinstate, and Remove. */
export type EntrantButtons = { confirm: boolean; drop: 'drop' | 'reinstate' | null; remove: 'remove' | null };

/** entrantButtons decides an entrant's buttons: Confirm while pending; Reinstate once dropped, and Drop only for one who has played; Remove only for one who has not (it takes them out of their round). */
export function entrantButtons(e: { id: number; status: AdminBlitzEntrant['status'] }, rounds: readonly AdminBlitzRound[]): EntrantButtons {
  const played = hasPlayed(e.id, rounds);
  return {
    confirm: e.status === 'pending',
    drop: e.status === 'dropped' ? 'reinstate' : e.status === 'active' && played ? 'drop' : null,
    remove: played ? null : 'remove',
  };
}

/** leftUnpaired lists whom removing an entrant leaves unpaired: their opponents in the matches still to play, oldest round first. */
export function leftUnpaired(id: number, rounds: readonly AdminBlitzRound[]): number[] {
  return [...rounds]
    .sort((a, b) => a.number - b.number)
    .flatMap((r) => r.matches.filter((m) => m.result === null && m.b !== null && (m.a === id || m.b === id)).map((m) => (m.a === id ? (m.b as number) : m.a)));
}

/** replaceableSide is the player who dropped from a knockout match still to play, whom Replace puts someone in place of; null for none. */
export function replaceableSide(m: AdminBlitzMatch, round: AdminBlitzRound, entrants: readonly { id: number; status: AdminBlitzEntrant['status'] }[]): number | null {
  if (round.kind !== 'knockout' || m.result !== null || m.b === null) return null;
  const dropped = (id: number) => entrants.some((e) => e.id === id && e.status === 'dropped');
  if (dropped(m.a)) return m.a;
  return dropped(m.b) ? m.b : null;
}

/** unpaired lists the active entrants a round leaves out, in sign-up order. */
export function unpaired(entrants: readonly AdminBlitzEntrant[], round: { matches: readonly AdminBlitzMatch[] }): AdminBlitzEntrant[] {
  const paired = new Set<number>();
  for (const m of round.matches) {
    paired.add(m.a);
    if (m.b !== null) paired.add(m.b);
  }
  return entrants.filter((e) => e.status === 'active' && !paired.has(e.id));
}

/** partnersFor lists whom a player may be paired with by hand in a Swiss round: the unpaired players but them and anyone they met in another Swiss round (a pairing never repeats); everyone unpaired for nobody picked. */
export function partnersFor(id: number | null, free: readonly AdminBlitzEntrant[], rounds: readonly AdminBlitzRound[], roundId: number): AdminBlitzEntrant[] {
  if (id === null) return [...free];
  const met = new Set(
    rounds
      .filter((r) => r.kind === 'swiss' && r.id !== roundId)
      .flatMap((r) => r.matches.filter((m) => m.b !== null && (m.a === id || m.b === id)).map((m) => (m.a === id ? (m.b as number) : m.a))),
  );
  return free.filter((e) => e.id !== id && !met.has(e.id));
}

/** byeTakers lists who may take the bye by hand: the unpaired players who have not had one in another round, or all of them when everyone has. */
export function byeTakers(free: readonly AdminBlitzEntrant[], rounds: readonly AdminBlitzRound[], roundId: number): AdminBlitzEntrant[] {
  const had = new Set(rounds.filter((r) => r.id !== roundId).flatMap((r) => r.matches.filter((m) => m.b === null).map((m) => m.a)));
  const fresh = free.filter((e) => !had.has(e.id));
  return fresh.length > 0 ? fresh : [...free];
}

/** pairCounts counts a round's players: those in a match or with the bye, and the active entrants it leaves out. */
export function pairCounts(entrants: readonly AdminBlitzEntrant[], matches: readonly AdminBlitzMatch[]): { paired: number; unpaired: number } {
  const paired = new Set(matches.flatMap((m) => (m.b === null ? [m.a] : [m.a, m.b])));
  return { paired: paired.size, unpaired: unpaired(entrants, { matches }).length };
}

/** asChanges turns matches into what the API takes back: entrant ids, b left out for the bye. */
export function asChanges(matches: readonly AdminBlitzMatch[]): MatchChange[] {
  return matches.map((m) => (m.b === null ? { a: m.a } : { a: m.a, b: m.b }));
}

/** zoneGapLabel says how far apart two players' clocks are. */
export function zoneGapLabel(minutes: number): string {
  if (minutes === 0) return 'same hour';
  const h = Math.floor(minutes / 60);
  const m = minutes % 60;
  return m === 0 ? `${h} h apart` : `${h} h ${m} apart`;
}
