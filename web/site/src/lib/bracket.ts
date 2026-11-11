/** A season shown as a bracket: its rounds named, the rounds still to come, the tree, and a player's road. */
import { isSlug, type BracketGroup, type BracketSeat, type SeasonBracket } from '@ttac/shared/ratings';

/** ViewRound is one round as the bracket shows it: named, and whether it is finished, being played or still to come. */
export type ViewRound = { name: string; short: string; state: 'finished' | 'in_play' | 'to_come'; groups: BracketGroup[] };

/** TREE_MOST is the most groups a round of the tree may have, so it fits a wide screen. */
const TREE_MOST = 8;

const LATE = [
  { groups: 4, name: 'Quarter-finals', short: 'QF' },
  { groups: 2, name: 'Semi-finals', short: 'SF' },
  { groups: 1, name: 'Final', short: 'F' },
];

/**
 * bracketRounds names a bracket's rounds and adds those still to come: where the last round has an even number of groups,
 * empty rounds of half as many, down to one. A bracket that ends in one group names its last three rounds by their
 * size (Quarter-finals, Semi-finals, Final); every other round is numbered.
 */
export function bracketRounds(bracket: SeasonBracket): ViewRound[] {
  const sizes = bracket.rounds.map((r) => r.groups.length);
  let last = sizes[sizes.length - 1] ?? 0;
  const rounds = bracket.rounds.map((r) => r.groups);
  while (last > 1 && last % 2 === 0) {
    last /= 2;
    rounds.push(Array.from({ length: last }, (_, i) => ({ name: `group ${i + 1}`, number: i + 1, state: 'never_played' as const, seats: [] })));
  }
  const toFinal = rounds[rounds.length - 1]?.length === 1;
  return rounds.map((groups, i) => {
    const fromEnd = rounds.length - 1 - i;
    const late = toFinal && fromEnd < LATE.length ? LATE[LATE.length - 1 - fromEnd] : undefined;
    if (late !== undefined && late.groups !== groups.length) return numbered(groups, i);
    return late === undefined ? numbered(groups, i) : { ...numbered(groups, i), name: late.name, short: late.short };
  });
}

/** numbered is round i (from 0) of the groups, named by its number. */
function numbered(groups: BracketGroup[], i: number): ViewRound {
  const state = groups.every((g) => g.seats.length === 0) ? 'to_come' : groups.every((g) => g.state === 'played') ? 'finished' : 'in_play';
  return { name: `Round ${i + 1}`, short: `R${i + 1}`, state, groups };
}

/**
 * treeStart returns the first round the bracket can show as a tree, up to the final: from it each round has at most
 * TREE_MOST groups, half as many as the round before, and every group's winner went on to the group its line leads
 * to (group n's to group ⌈n/2⌉). It returns rounds.length when fewer than two rounds qualify.
 */
export function treeStart(rounds: ViewRound[]): number {
  let start = rounds.length;
  for (let i = rounds.length - 1; i >= 0; i--) {
    const r = rounds[i];
    const next = rounds[i + 1];
    if (r === undefined) break;
    const fits = r.groups.length <= TREE_MOST && (next === undefined || next.groups.length * 2 === r.groups.length);
    const followed = r.groups.every((g) => g.seats.every((s) => s.place !== 1 || s.next === 0 || s.next === Math.ceil(g.number / 2)));
    if (!fits || !followed) break;
    start = i;
  }
  return rounds.length - start < 2 ? rounds.length : start;
}

/** SeatView is how a seat shows: through to the next round (or champion), knocked out, where they play next, and their points. */
export type SeatView = { through: boolean; out: boolean; to: string; points: string };

/** seatView says how a seat of a round's group shows: through when found in the next round, or in a finished group's top half. */
export function seatView(rounds: ViewRound[], r: number, group: BracketGroup, seat: BracketSeat): SeatView {
  const played = group.state === 'played';
  const final = r === rounds.length - 1;
  const champion = final && played && seat.place === 1 && rounds[r]?.groups.length === 1;
  const next = rounds[r + 1];
  const through = seat.next > 0 || champion || (played && !final && seat.place >= 1 && seat.place <= group.seats.length / 2);
  return {
    through,
    out: played && !final && !through,
    to: seat.next > 0 && next !== undefined ? `→ ${next.short} ${seat.next}` : champion ? 'Champion' : '',
    points: played && seat.points !== undefined ? pointsText(seat.points) : '',
  };
}

/** pointsText writes points as a whole number or to one place, with a true minus sign. */
function pointsText(points: number): string {
  const text = Number.isInteger(points) ? String(Math.abs(points)) : Math.abs(points).toFixed(1);
  return points < 0 ? `−${text}` : text;
}

const ORDINALS = ['1st', '2nd', '3rd', '4th', '5th', '6th'];

/** RoadStep is one group of a player's road: the round (final when it has one group), the group, the player's seat and how it went. */
export type RoadStep = { round: string; final: boolean; group: BracketGroup; seat: BracketSeat; result: string };

/** road follows a player (by entrant) through the rounds they played, with each result. */
export function road(rounds: ViewRound[], entrant: number): RoadStep[] {
  const steps: RoadStep[] = [];
  rounds.forEach((r, i) => {
    for (const g of r.groups) {
      const s = g.seats.find((x) => x.entrant === entrant);
      if (s === undefined) continue;
      const view = seatView(rounds, i, g, s);
      let result = g.state === 'running' ? 'Being played' : 'To play';
      if (g.state === 'played') {
        const place = view.to === 'Champion' ? 'Champion' : (ORDINALS[s.place - 1] ?? `${s.place}th`);
        result = view.points === '' ? place : `${place} · ${view.points} pts`;
      }
      steps.push({ round: r.name, final: r.groups.length === 1, group: g, seat: s, result });
    }
  });
  return steps;
}

/** bracketMap marks, round by round, which group a player (by entrant) was in. */
export function bracketMap(rounds: ViewRound[], entrant: number): { name: string; cells: boolean[] }[] {
  return rounds.map((r) => ({ name: r.name, cells: r.groups.map((g) => g.seats.some((s) => s.entrant === entrant)) }));
}

/** defaultRound is the round a bracket opens on: its latest with players. */
export function defaultRound(rounds: ViewRound[]): number {
  for (let i = rounds.length - 1; i > 0; i--) {
    if (rounds[i]?.state !== 'to_come') return i;
  }
  return 0;
}

/** groupsMatching keeps the groups with a player whose name holds q, in any case; all of them for nothing typed. */
export function groupsMatching(groups: BracketGroup[], q: string): BracketGroup[] {
  const want = q.trim().toLowerCase();
  if (want === '') return groups;
  return groups.filter((g) => g.seats.some((s) => s.name.toLowerCase().includes(want)));
}

/** bracketHref is the address of every bracket, of a tournament's (its latest season), or of one season's. */
export function bracketHref(slug?: string, season?: string): string {
  if (slug === undefined) return '/ratings/brackets/';
  const q = new URLSearchParams({ t: slug });
  if (season !== undefined) q.set('s', season);
  return `/ratings/brackets/?${q}`;
}

/** bracketParams reads the tournament (?t=) and season (?s=) an address names, null for none or one the API would refuse. */
export function bracketParams(search: URLSearchParams): { slug: string | null; season: string | null } {
  const t = search.get('t');
  const s = search.get('s');
  return { slug: t !== null && isSlug(t) ? t : null, season: s !== null && s !== '' && s.length <= 64 ? s : null };
}

/** openingSeason is the season a bracket opens on: the one asked for when it has games, else the latest with games. */
export function openingSeason(seasons: readonly { label: string; games: number }[], asked: string | null): string | null {
  const played = seasons.filter((s) => s.games > 0);
  if (asked !== null && played.some((s) => s.label === asked)) return asked;
  return played[played.length - 1]?.label ?? null;
}
