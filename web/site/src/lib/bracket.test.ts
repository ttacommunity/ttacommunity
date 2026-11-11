import { describe, expect, it } from 'vitest';
import type { BracketGroup, BracketSeat, SeasonBracket } from '@ttac/shared/ratings';
import { bracketHref, bracketParams, defaultRound, openingSeason, bracketMap, bracketRounds, groupsMatching, road, seatView, treeStart, type ViewRound } from './bracket';

/** at returns xs[i], failing the test when there is none. */
function at<T>(xs: readonly T[], i: number): T {
  const x = xs[i];
  if (x === undefined) throw new Error(`no item ${i}`);
  return x;
}

const seat = (entrant: number, name: string, place = 0, next = 0, points?: number): BracketSeat => ({ entrant, name, place, next, points });
const played = (number: number, seats: BracketSeat[]): BracketGroup => ({ name: `group ${number}`, number, state: 'played', seats });
const bracket = (...rounds: BracketGroup[][]): SeasonBracket => ({
  run_id: 1,
  slug: 'grand_open',
  season: '2026',
  rounds: rounds.map((groups, i) => ({ stage: `Stage ${i + 1}`, groups })),
});

// Four groups, then two, then the final: a whole bracket of 16 players, as the API gives it.
const whole = bracket(
  [
    played(1, [seat(1, 'Aster', 1, 1, 6), seat(2, 'Brook', 2, 2, 3), seat(3, 'Cove', 3, 0, 1), seat(4, 'Dune', 4, 0, 0)]),
    played(2, [seat(5, 'Ember', 1, 1, 6), seat(6, 'Fern', 2, 2, 3), seat(7, 'Gale', 3, 0, 1), seat(8, 'Heath', 4, 0, -1)]),
    played(3, [seat(9, 'Iris', 1, 2, 6), seat(10, 'Jade', 2, 1, 3), seat(11, 'Kestrel', 3, 0, 1), seat(12, 'Lark', 4, 0, 0)]),
    played(4, [seat(13, 'Moss', 1, 2, 6), seat(14, 'Nettle', 2, 1, 3), seat(15, 'Oak', 3, 0, 1), seat(16, 'Pine', 4, 0, 0)]),
  ],
  [
    played(1, [seat(1, 'Aster', 1, 1, 6), seat(5, 'Ember', 2, 1, 3), seat(10, 'Jade', 3, 0, 1), seat(14, 'Nettle', 4, 0, 0)]),
    played(2, [seat(9, 'Iris', 1, 1, 6), seat(13, 'Moss', 2, 1, 3), seat(2, 'Brook', 3, 0, 1), seat(6, 'Fern', 4, 0, 0)]),
  ],
  [played(1, [seat(9, 'Iris', 1, 0, 6), seat(1, 'Aster', 2, 0, 3), seat(13, 'Moss', 3, 0, 1), seat(5, 'Ember', 4, 0, 0)])],
);

// The same bracket with its second round being played and no final yet.
const semi = at(whole.rounds, 1).groups;
const inPlay = bracket(at(whole.rounds, 0).groups, [{ ...at(semi, 0), state: 'running', seats: at(semi, 0).seats.map((s) => seat(s.entrant, s.name)) }, at(semi, 1)]);

describe('bracketRounds', () => {
  it('names the last three rounds of a bracket that halves to a final, and the others by number', () => {
    const big = bracket(...[16, 8, 4, 2, 1].map((n) => Array.from({ length: n }, (_, i) => played(i + 1, []))));
    expect(bracketRounds(big).map((r) => `${r.name} (${r.short})`)).toEqual([
      'Round 1 (R1)',
      'Round 2 (R2)',
      'Quarter-finals (QF)',
      'Semi-finals (SF)',
      'Final (F)',
    ]);
  });

  it('adds the rounds still to come, down to the final, as empty groups', () => {
    const got = bracketRounds(inPlay);
    expect(got.map((r) => [r.name, r.state, r.groups.length])).toEqual([
      ['Quarter-finals', 'finished', 4],
      ['Semi-finals', 'in_play', 2],
      ['Final', 'to_come', 1],
    ]);
    expect(at(at(got, 2).groups, 0).seats).toEqual([]);
  });

  it('names rounds by number when the bracket does not end in one group', () => {
    const odd = bracket([played(1, []), played(2, []), played(3, [])], [played(1, [])]);
    expect(bracketRounds(odd).map((r) => r.name)).toEqual(['Round 1', 'Final']);
    const uneven = bracket([played(1, []), played(2, []), played(3, [])]);
    expect(bracketRounds(uneven).map((r) => r.name)).toEqual(['Round 1']);
  });
});

describe('treeStart', () => {
  const rounds = (groups: number[]) =>
    bracketRounds(bracket(...groups.map((n) => Array.from({ length: n }, (_, i) => played(i + 1, [seat(i * 4 + 1, `P${i}`, 1, Math.ceil((i + 1) / 2))])))));

  it.each([
    ['a whole bracket whose winners follow the lines', [4, 2, 1], 0],
    ['a big bracket, from its round of eight groups', [64, 32, 16, 8, 4, 2, 1], 3],
    ['a single group', [1], 1],
  ])('brackets %s as a tree', (_name, groups, want) => {
    expect(treeStart(rounds(groups))).toBe(want);
  });

  it('starts the tree after a round whose winners do not follow its lines', () => {
    const crossed = bracketRounds(structuredClone(whole));
    const first = at(at(crossed, 0).groups, 0);
    first.seats[0] = { ...at(first.seats, 0), next: 2 };
    expect(treeStart(crossed)).toBe(1);
  });
});

describe('seatView', () => {
  const rounds = bracketRounds(whole);
  it.each([
    ['a winner going through', 0, 0, 0, { through: true, out: false, to: '→ SF 1', points: '6' }],
    ['a runner-up going through', 0, 2, 1, { through: true, out: false, to: '→ SF 1', points: '3' }],
    ['a player knocked out', 0, 1, 3, { through: false, out: true, to: '', points: '−1' }],
    ['the champion', 2, 0, 0, { through: true, out: false, to: 'Champion', points: '6' }],
    ['the runner-up of the final', 2, 0, 1, { through: false, out: false, to: '', points: '3' }],
  ] as const)('shows %s', (_name, r, g, s, want) => {
    const group = at(at(rounds, r).groups, g);
    expect(seatView(rounds, r, group, at(group.seats, s))).toEqual(want);
  });

  it.each([
    ['its winner through', 0, { through: true, out: false, to: '', points: '6' }],
    ['its runner-up through', 1, { through: true, out: false, to: '', points: '3' }],
    ['its third out', 2, { through: false, out: true, to: '', points: '1' }],
  ])('shows a finished group whose next round is still to come with %s', (_name, s, want) => {
    const first = bracket(at(whole.rounds, 0).groups.map((g) => ({ ...g, seats: g.seats.map((x) => ({ ...x, next: 0 })) })));
    const waiting = bracketRounds(first);
    const group = at(at(waiting, 0).groups, 0);
    expect(seatView(waiting, 0, group, at(group.seats, s))).toEqual(want);
  });

  it.each([
    ['its winner through', 0, { through: true, out: false, to: '', points: '1' }],
    ['its loser out', 1, { through: false, out: true, to: '', points: '0' }],
  ])('shows a finished two-player match whose next round is still to come with %s', (_name, s, want) => {
    const matches = bracketRounds(bracket([played(1, [seat(1, 'Aster', 1, 0, 1), seat(2, 'Brook', 2, 0, 0)]), played(2, [seat(3, 'Cove', 1, 0, 1), seat(4, 'Dune', 2, 0, 0)])]));
    const group = at(at(matches, 0).groups, 0);
    expect(seatView(matches, 0, group, at(group.seats, s))).toEqual(want);
  });

  it('shows a group still being played without results', () => {
    const live = bracketRounds(inPlay);
    const group = at(at(live, 1).groups, 0);
    expect(seatView(live, 1, group, at(group.seats, 0))).toEqual({ through: false, out: false, to: '', points: '' });
  });
});

describe('road', () => {
  it("follows a player's groups to the end, with each result", () => {
    expect(road(bracketRounds(whole), 9).map((s) => [s.round, s.group.number, s.result])).toEqual([
      ['Quarter-finals', 3, '1st · 6 pts'],
      ['Semi-finals', 2, '1st · 6 pts'],
      ['Final', 1, 'Champion · 6 pts'],
    ]);
  });

  it('stops where a player went out, and says when a group is still being played', () => {
    expect(road(bracketRounds(whole), 16).map((s) => s.result)).toEqual(['4th · 0 pts']);
    expect(road(bracketRounds(inPlay), 1).map((s) => s.result)).toEqual(['1st · 6 pts', 'Being played']);
  });
});

describe('bracketMap', () => {
  it("lights the player's group in every round, and none where they did not play", () => {
    expect(bracketMap(bracketRounds(whole), 13)).toEqual([
      { name: 'Quarter-finals', cells: [false, false, false, true] },
      { name: 'Semi-finals', cells: [false, true] },
      { name: 'Final', cells: [true] },
    ]);
    expect(bracketMap(bracketRounds(whole), 16).map((r) => r.cells.some(Boolean))).toEqual([true, false, false]);
  });
});

describe('defaultRound', () => {
  it.each([
    ['a finished bracket', whole, 2],
    ['a bracket being played', inPlay, 1],
  ])('opens %s on its latest round with players', (_name, d, want) => {
    expect(defaultRound(bracketRounds(d))).toBe(want);
  });
});

describe('groupsMatching', () => {
  const groups: ViewRound['groups'] = at(whole.rounds, 0).groups;
  it.each([
    ['nothing typed', '', [1, 2, 3, 4]],
    ['part of a name, in any case', 'EST', [3]],
    ['a name in no group', 'zinnia', []],
  ])('keeps the groups for %s', (_name, q, want) => {
    expect(groupsMatching(groups, q).map((g) => g.number)).toEqual(want);
  });
});

describe('bracketHref and bracketParams', () => {
  it.each([
    ['every bracket', undefined, undefined, '/ratings/brackets/'],
    ["a tournament's latest season", 'australian_open', undefined, '/ratings/brackets/?t=australian_open'],
    ['a season with a space', 'australian_open', '2020 Play Off', '/ratings/brackets/?t=australian_open&s=2020+Play+Off'],
  ])('link to %s', (_name, slug, season, want) => {
    expect(bracketHref(slug, season)).toBe(want);
  });

  it('read a tournament and season back from the address', () => {
    expect(bracketParams(new URLSearchParams('t=australian_open&s=2020+Play+Off'))).toEqual({ slug: 'australian_open', season: '2020 Play Off' });
    expect(bracketParams(new URLSearchParams('t=australian_open'))).toEqual({ slug: 'australian_open', season: null });
  });

  it.each([
    ['nothing', ''],
    ['a tournament the API would refuse', 't=Australian+Open'],
    ['a season too long for the API', `t=australian_open&s=${'x'.repeat(65)}`],
  ])('read no tournament, or no season, from %s', (_name, q) => {
    const got = bracketParams(new URLSearchParams(q));
    expect(got.slug === null || got.season === null).toBe(true);
  });
});

describe('openingSeason', () => {
  const seasons = [{ label: '2024', games: 127 }, { label: '2025', games: 127 }, { label: '2026', games: 0 }];
  it.each([
    ['the season asked for', '2024', '2024'],
    ['the latest with games when none is asked for', null, '2025'],
    ['the latest with games for a season without any', '2026', '2025'],
    ['the latest with games for a season it does not have', '1999', '2025'],
  ])('opens %s', (_name, asked, want) => {
    expect(openingSeason(seasons, asked)).toBe(want);
  });

  it('opens nothing for a tournament without games', () => {
    expect(openingSeason([{ label: '2026', games: 0 }], null)).toBeNull();
  });
});
