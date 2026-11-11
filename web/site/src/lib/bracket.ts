/** A season shown as a bracket: its rounds named, the rounds still to come, the tree, and a player's road. */
import { isSlug, type BlitzEditionRow, type BracketGroup, type BracketSeat, type SeasonBracket } from '@ttac/shared/ratings';
import { editionHref } from '@ttac/shared/blitz';

/**
 * ViewRound is one round as the bracket shows it: named, whether it is finished, being played or still to come, and
 * what its groups are called (a blitz knockout's qualifiers by Seed, its matches Match, else Group).
 */
export type ViewRound = { name: string; short: string; state: 'finished' | 'in_play' | 'to_come'; unit: 'Seed' | 'Match' | 'Group'; groups: BracketGroup[] };

/** TREE_MOST is the most groups the first round of the tree a bracket opens on may have, so it fits a wide screen. */
const TREE_MOST = 8;

const LATE = [
  { groups: 4, name: 'Quarter-finals', short: 'QF' },
  { groups: 2, name: 'Semi-finals', short: 'SF' },
  { groups: 1, name: 'Final', short: 'F' },
];

/**
 * bracketRounds names a bracket's rounds and adds those still to come: where the last round has an even number of groups,
 * empty rounds of half as many, down to one. A blitz knockout's first round is its Qualifiers. A bracket that ends in
 * one group names its last three rounds by their size (Quarter-finals, Semi-finals, Final); every other round is
 * numbered, the qualifiers aside.
 */
export function bracketRounds(bracket: SeasonBracket): ViewRound[] {
  const sizes = bracket.rounds.map((r) => r.groups.length);
  let last = sizes[sizes.length - 1] ?? 0;
  const rounds = bracket.rounds.map((r) => r.groups);
  while (last > 1 && last % 2 === 0) {
    last /= 2;
    rounds.push(Array.from({ length: last }, (_, i) => ({ name: `group ${i + 1}`, number: i + 1, state: 'never_played' as const, seats: [] })));
  }
  const qualified = bracket.rounds[0]?.qualifiers === true ? 1 : 0;
  const unit = qualified === 1 ? 'Match' : 'Group';
  const toFinal = rounds[rounds.length - 1]?.length === 1;
  return rounds.map((groups, i) => {
    if (i < qualified) return { ...numbered(groups, 0, 'Seed'), name: 'Qualifiers', short: 'Q' };
    const fromEnd = rounds.length - 1 - i;
    const late = toFinal && fromEnd < LATE.length ? LATE[LATE.length - 1 - fromEnd] : undefined;
    const round = numbered(groups, i - qualified, unit);
    return late === undefined || late.groups !== groups.length ? round : { ...round, name: late.name, short: late.short };
  });
}

/** numbered is round i (from 0) of the groups, named by its number. */
function numbered(groups: BracketGroup[], i: number, unit: ViewRound['unit']): ViewRound {
  const state = groups.every((g) => g.seats.length === 0) ? 'to_come' : groups.every((g) => g.state === 'played') ? 'finished' : 'in_play';
  return { name: `Round ${i + 1}`, short: `R${i + 1}`, state, unit, groups };
}

/** groupLabel names a group of round r: a qualifier by their seed, a round's only group by the round, else by its number (after the round's short name in the tree). */
export function groupLabel(rounds: ViewRound[], r: number, group: BracketGroup, titled: boolean): string {
  const x = rounds[r];
  if (x === undefined) return '';
  if (x.unit === 'Seed') return `Seed ${group.seats[0]?.seed ?? group.number}`;
  if (x.groups.length === 1) return x.name;
  return titled ? `${x.short} · ${x.unit} ${group.number}` : `${x.unit} ${group.number}`;
}

/** seedLabel writes a seat's seed as (3), or nothing for a seat without one. */
export function seedLabel(seat: BracketSeat): string {
  return seat.seed === undefined ? '' : `(${seat.seed})`;
}

/**
 * treeReach returns the first round the bracket can draw as a tree to its end: from it each round has half the groups of
 * the one before, and everyone who goes on goes to the group their line leads to (group n's to group ⌈n/2⌉). It returns
 * rounds.length when fewer than two rounds can.
 */
export function treeReach(rounds: ViewRound[]): number {
  let reach = rounds.length - 1;
  for (let i = rounds.length - 2; i >= 0; i--) {
    const r = rounds[i];
    const next = rounds[i + 1];
    if (r === undefined || next === undefined || next.groups.length * 2 !== r.groups.length) break;
    if (r.groups.some((g) => g.seats.some((s) => s.next > 0 && s.next !== Math.ceil(g.number / 2)))) break;
    reach = i;
  }
  return reach >= rounds.length - 1 ? rounds.length : reach;
}

/** BracketView is the rounds shown, first to last, and a round tapped alone, which a second tap makes one end of a range. */
export type BracketView = { first: number; last: number; anchor: number | null };

/** defaultView is what a bracket opens on: its tree from the first round it can draw with at most TREE_MOST groups, to its end; without a tree, its latest round with players. */
export function defaultView(rounds: ViewRound[]): BracketView {
  const reach = treeReach(rounds);
  const last = rounds.length - 1;
  if (reach >= rounds.length) {
    const r = defaultRound(rounds);
    return { first: r, last: r, anchor: null };
  }
  let first = reach;
  while (first < last - 1 && (rounds[first]?.groups.length ?? 0) > TREE_MOST) first++;
  return { first, last, anchor: null };
}

/** tapRound is the view after tapping round i: that round alone, or, tapped after another round alone, the two and those between. */
export function tapRound(view: BracketView, i: number): BracketView {
  if (view.anchor !== null && view.anchor !== i) return { first: Math.min(view.anchor, i), last: Math.max(view.anchor, i), anchor: null };
  return { first: i, last: i, anchor: i };
}

/**
 * shownRounds is the rounds drawn side by side, start to end, and for each whether lines join its groups to the next
 * round's (from reach, where the tree holds, to the last shown): a range as chosen, or one round shown as a tree beside
 * the next once that has players. Null for a round shown alone, as a grid.
 */
export function shownRounds(rounds: ViewRound[], view: BracketView, reach: number, single: 'tree' | 'grid'): { start: number; end: number; joined: boolean[] } | null {
  let end = view.last;
  if (view.first === end) {
    if (single === 'grid') return null;
    const next = rounds[end + 1];
    if (next === undefined || next.state === 'to_come') return null;
    end++;
  }
  const joined = rounds.slice(view.first, end + 1).map((r, k) => view.first + k >= reach && view.first + k < end && r.groups.length > 1);
  return { start: view.first, end, joined };
}

/**
 * bracketGrid lays rounds side by side on shared rows as a tree, whether or not they halve or lines join them: the kth
 * round's groups span 2^k rows each, so each sits between two of the round before. A joined round's groups go in pairs,
 * each spanning the rows of the group it feeds. Returns each column's rows per item and items, and the rows in all.
 */
export function bracketGrid(rounds: BracketGroup[][], joined: boolean[]): { rows: number; columns: { span: number; items: BracketGroup[][] }[] } {
  const columns = rounds.map((groups, k) => {
    const pairs = joined[k] === true;
    const items = pairs ? groups.flatMap((g, j) => (j % 2 === 0 ? [groups.slice(j, j + 2)] : [])) : groups.map((g) => [g]);
    return { span: 2 ** (pairs ? k + 1 : k), items };
  });
  return { rows: Math.max(0, ...columns.map((c) => c.span * c.items.length)), columns };
}

/** SeatView is how a seat shows: through to the next round (or champion), knocked out, where they play next, and their points. */
export type SeatView = { through: boolean; out: boolean; to: string; points: string };

/** seatView says how a seat of a round's group shows: through when found in the next round, or, while that round is to
 * come, in a finished group's top half; the champion is a played final's first. A played group's players come best first,
 * so their order is their place, also where a group of several games gives two players a best place of 1. */
export function seatView(rounds: ViewRound[], r: number, group: BracketGroup, seat: BracketSeat): SeatView {
  const played = group.state === 'played';
  const final = r === rounds.length - 1;
  const rank = group.seats.findIndex((x) => x.entrant === seat.entrant) + 1;
  const champion = final && played && rank === 1 && rounds[r]?.groups.length === 1;
  const next = rounds[r + 1];
  const waiting = next === undefined || next.state === 'to_come';
  const through = seat.next > 0 || champion || (played && !final && waiting && rank >= 1 && rank <= group.seats.length / 2);
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

/** RoadStep is one group of a player's road: the round (final when it has one group), the group and its label, the player's seat and how it went. */
export type RoadStep = { round: string; final: boolean; group: BracketGroup; label: string; seat: BracketSeat; result: string };

/** road follows a player (by entrant) through the rounds they played, with each result. */
export function road(rounds: ViewRound[], entrant: number): RoadStep[] {
  const steps: RoadStep[] = [];
  rounds.forEach((r, i) => {
    for (const g of r.groups) {
      const s = g.seats.find((x) => x.entrant === entrant);
      if (s === undefined) continue;
      const view = seatView(rounds, i, g, s);
      let result = g.state === 'running' ? 'Being played' : 'To play';
      if (r.unit === 'Seed') {
        result = `Seed ${s.seed ?? g.number}`;
      } else if (g.state === 'played') {
        const rank = g.seats.indexOf(s) + 1;
        const place = view.to === 'Champion' ? 'Champion' : (ORDINALS[rank - 1] ?? `${rank}th`);
        result = view.points === '' ? place : `${place} · ${view.points} pts`;
      }
      const final = r.groups.length === 1;
      const label = final ? (r.unit === 'Group' ? 'The final group' : 'The final') : groupLabel(rounds, i, g, false);
      steps.push({ round: r.name, final, group: g, label, seat: s, result });
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

/** roadsKept is the groups to keep when q names a player in full: theirs and every opponent's, as round/group keys; null otherwise. */
export function roadsKept(rounds: ViewRound[], q: string): Set<string> | null {
  const want = q.trim().toLowerCase();
  const seats = rounds.flatMap((r, i) => r.groups.flatMap((g) => g.seats.map((s) => ({ key: `${i}/${g.number}`, group: g, s }))));
  const player = seats.find((x) => x.s.name.toLowerCase() === want)?.s.entrant;
  if (want === '' || player === undefined) return null;
  const met = new Set(seats.filter((x) => x.s.entrant === player).flatMap((x) => x.group.seats.map((s) => s.entrant)));
  return new Set(seats.filter((x) => met.has(x.s.entrant)).map((x) => x.key));
}

/** CompactItem is a group kept in a squashed tree: its rows (start to end), the rows its line in reaches (from the first group feeding it to the last), and whether a line goes out to the group it feeds. */
export type CompactItem = { group: BracketGroup; start: number; end: number; bar: [number, number] | null; out: boolean };

/**
 * compactTree squashes the rounds start to end to the groups kept, two rows each, stacked from the top; where joined
 * says lines join a round to the next, each group of the next spans the groups that fed it (by where their players
 * went next) and its line in reaches from the first to the last.
 */
export function compactTree(rounds: ViewRound[], start: number, end: number, kept: Set<string>, joined: boolean[]): { rows: number; columns: { items: CompactItem[] }[] } {
  const centre = (x: CompactItem) => (x.start + x.end) / 2;
  const feeds = (from: CompactItem, to: BracketGroup) => from.group.seats.some((s) => s.next === to.number);
  const columns: { items: CompactItem[] }[] = [];
  rounds.slice(start, end + 1).forEach((r, k) => {
    const prev = columns[k - 1]?.items ?? [];
    const lines = joined[k - 1] === true;
    let bottom = 0;
    const items = r.groups
      .filter((g) => kept.has(`${start + k}/${g.number}`))
      .map((group): CompactItem => {
        const fed = lines ? prev.filter((p) => feeds(p, group)) : [];
        let from = fed.length > 0 ? Math.min(...fed.map((p) => p.start)) : bottom;
        let to = fed.length > 0 ? Math.max(...fed.map((p) => p.end)) : bottom + 2;
        if (from < bottom) [from, to] = [bottom, to + bottom - from];
        bottom = to;
        const bar: [number, number] | null = fed.length > 0 ? [Math.min(...fed.map(centre)), Math.max(...fed.map(centre))] : null;
        return { group, start: from, end: to, bar, out: false };
      });
    if (lines) prev.forEach((p) => (p.out = items.some((x) => feeds(p, x.group))));
    columns.push({ items });
  });
  return { rows: Math.max(0, ...columns.flatMap((c) => c.items.map((x) => x.end))), columns };
}

/** bracketPlayers is each player of the bracket once, by name, for the search box to suggest. */
export function bracketPlayers(rounds: ViewRound[]): { name: string }[] {
  const names = new Set(rounds.flatMap((r) => r.groups.flatMap((g) => g.seats.map((s) => s.name))));
  return [...names].sort((a, b) => a.localeCompare(b)).map((name) => ({ name }));
}

/** allView shows every round. */
export function allView(rounds: ViewRound[]): BracketView {
  return { first: 0, last: Math.max(rounds.length - 1, 0), anchor: null };
}

/** groupsMatching keeps the groups with a player whose name holds q, in any case; all of them for nothing typed. */
export function groupsMatching(groups: BracketGroup[], q: string): BracketGroup[] {
  const want = q.trim().toLowerCase();
  if (want === '') return groups;
  return groups.filter((g) => g.seats.some((s) => s.name.toLowerCase().includes(want)));
}

/** bracketCard is where a tournament's card on the Brackets list leads: a blitz tournament whose latest edition is in
 * its Swiss rounds to that edition's standings, every other to its bracket. */
export function bracketCard(slug: string, editions: readonly BlitzEditionRow[]): { href: string; cta: string } {
  const latest = editions.find((e) => e.tournament === slug);
  if (latest !== undefined && !latest.knockout) return { href: editionHref(slug, latest.edition), cta: 'Swiss rounds: see the standings' };
  return { href: bracketHref(slug), cta: 'View the bracket' };
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

/** ZOOMS are the tree's sizes, from the smallest. */
const ZOOMS = [0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1, 1.25, 1.5];

/** fitZoom is the size that fits a tree of treeWidth into a box of boxWidth: never larger than full size or smaller than the least; full size before the box is measured. */
export function fitZoom(boxWidth: number, treeWidth: number): number {
  if (boxWidth <= 0 || treeWidth <= 0) return 1;
  return Math.min(1, Math.max(ZOOMS[0] ?? 0.4, boxWidth / treeWidth));
}

/** stepZoom is the next size in (dir 1) or out (dir −1) from zoom, staying at the largest or smallest. */
export function stepZoom(zoom: number, dir: 1 | -1): number {
  const next = dir === 1 ? ZOOMS.find((z) => z > zoom + 0.001) : [...ZOOMS].reverse().find((z) => z < zoom - 0.001);
  return next ?? zoom;
}

/** pinchZoom is the size for a pinch that began at size from with the fingers start apart and is now at now apart, kept between the smallest and largest sizes. */
export function pinchZoom(from: number, start: number, now: number): number {
  if (start <= 0) return from;
  return Math.min(ZOOMS[ZOOMS.length - 1] ?? 1.5, Math.max(ZOOMS[0] ?? 0.4, (from * now) / start));
}

/** anchoredScroll is the scroll that keeps a point offset px into the tree where it was when its size goes from from to to. */
export function anchoredScroll(scroll: number, offset: number, from: number, to: number): number {
  return scroll + offset * (to / from - 1);
}
