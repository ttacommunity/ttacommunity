import { describe, expect, it } from 'vitest';
import type { BlitzEditionRow, BracketGroup, BracketSeat, SeasonBracket } from '@ttac/shared/ratings';
import {
  bracketCard,
  bracketHref,
  anchoredScroll,
  fitZoom,
  pinchZoom,
  stepZoom,
  tapRound,
  shownRounds,
  treeReach,
  defaultView,
  bracketParams,
  defaultRound,
  groupLabel,
  openingSeason,
  bracketMap,
  bracketRounds,
  groupsMatching,
  road,
  seatView,
  seedLabel,
  type BracketView,
  type ViewRound,
} from './bracket';

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
  rounds: rounds.map((groups, i) => ({ stage: `Stage ${i + 1}`, qualifiers: false, groups })),
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

// A bracket of rounds of the sizes given, each group's winner going on to group ⌈n/2⌉, except the groups strays names.
const sized = (groups: number[], strays: Record<number, number> = {}) =>
  bracketRounds(
    bracket(
      ...groups.map((n, r) =>
        Array.from({ length: n }, (_, i) => played(i + 1, [seat(i * 4 + 1, `P${i}`, 1, r === groups.length - 1 ? 0 : i < (strays[r] ?? 0) ? Math.ceil((i + 1) / 2) + 1 : Math.ceil((i + 1) / 2))])),
      ),
    ),
  );

describe('treeReach', () => {
  it.each([
    ['a whole bracket whose winners follow the lines', sized([4, 2, 1]), 0],
    ['a big bracket, whatever its size', sized([64, 32, 16, 8, 4, 2, 1]), 0],
    ['a single group', sized([1]), 1],
    ['a bracket with one winner astray in its first round', sized([4, 2, 1], { 0: 1 }), 1],
    ['a bracket with one winner astray in a big round', sized([32, 16, 8, 4, 2, 1], { 1: 1 }), 2],
    ['a round that does not halve', sized([3, 2, 1]), 1],
  ])('reaches back to the first round it can draw for %s', (_name, rounds, want) => {
    expect(treeReach(rounds)).toBe(want);
  });

  it('draws no round whose runners-up go to the neighbouring group', () => {
    expect(treeReach(bracketRounds(whole))).toBe(1);
  });
});

describe('defaultView', () => {
  it.each([
    ['a big bracket, from its round of eight groups', sized([64, 32, 16, 8, 4, 2, 1]), { first: 3, last: 6 }],
    ['a bracket redrawn up to its quarter-finals, from them', sized([64, 32, 16, 8, 4, 2, 1], { 0: 40, 1: 20, 2: 10, 3: 5 }), { first: 4, last: 6 }],
    ['a small bracket, whole', sized([4, 2, 1]), { first: 0, last: 2 }],
    ['a bracket without a tree, on its latest round', sized([3, 1]), { first: 1, last: 1 }],
  ])('opens %s', (_name, rounds, want) => {
    expect(defaultView(rounds)).toMatchObject({ ...want, anchor: null });
  });

  it('opens a blitz knockout whole', () => {
    expect(defaultView(bracketRounds(blitz))).toMatchObject({ first: 0, last: 2 });
  });
});

describe('tapRound', () => {
  const opened: BracketView = { first: 3, last: 6, anchor: null };
  it.each([
    ['one round, which it shows alone', [4], { first: 4, last: 4 }],
    ['a second round, which shows the two and those between', [3, 6], { first: 3, last: 6 }],
    ['a second round before the first', [6, 3], { first: 3, last: 6 }],
    ['a third round, which starts again', [3, 6, 4], { first: 4, last: 4 }],
    ['the same round twice', [4, 4], { first: 4, last: 4 }],
    ['a round before the tree, alone', [1], { first: 1, last: 1 }],
    ['a round before the tree after one in it, alone', [4, 1], { first: 1, last: 1 }],
    ['a round in the tree after one before it, alone', [1, 5], { first: 5, last: 5 }],
  ])('chooses %s, the tree from round 3', (_name, taps, want) => {
    expect(taps.reduce((v, i) => tapRound(v, i, 3), opened)).toMatchObject(want);
  });
});

describe('shownRounds', () => {
  const rounds = sized([64, 32, 16, 8, 4, 2, 1]);
  it.each([
    ['several rounds as a tree', { first: 3, last: 6 }, 0, { start: 3, end: 6, joined: true }],
    ['one round before the tree beside the next, unjoined', { first: 2, last: 2 }, 5, { start: 2, end: 3, joined: false }],
    ['one round of the tree beside the next, joined', { first: 5, last: 5 }, 5, { start: 5, end: 6, joined: true }],
    ['the final alone', { first: 6, last: 6 }, 0, null],
  ])('shows %s', (_name, view, reach, want) => {
    expect(shownRounds(rounds, { ...view, anchor: null }, reach)).toEqual(want);
  });

  it('shows a round alone while the next is to come', () => {
    expect(shownRounds(bracketRounds(inPlay), { first: 1, last: 1, anchor: null }, 0)).toBeNull();
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

  it.each([
    ['its best on points the champion', 0, { through: true, to: 'Champion' }],
    ['the other, who also won a game, not', 1, { through: false, to: '' }],
  ])('shows a final of several games with %s', (_name, s, want) => {
    const final = bracketRounds(bracket([played(1, [seat(1, 'Aster', 1, 0, 3), seat(2, 'Brook', 1, 0, 2)])]));
    const group = at(at(final, 0).groups, 0);
    expect(seatView(final, 0, group, at(group.seats, s))).toMatchObject(want);
  });

  describe('a match played as several games, each player winning one', () => {
    const match = (next: number) => [played(1, [seat(1, 'Aster', 1, next, 3), seat(2, 'Brook', 1, 0, 1)]), played(2, [seat(3, 'Cove', 1, next, 3), seat(4, 'Dune', 2, 0, 0)])];
    it.each([
      ['its loser out, once the next round has them', bracket(match(1), [played(1, [seat(1, 'Aster', 1), seat(3, 'Cove', 2)])])],
      ['its loser out, while the next round is to come', bracket(match(0))],
    ])('shows %s', (_name, d) => {
      const rounds = bracketRounds(d);
      const group = at(at(rounds, 0).groups, 0);
      expect(seatView(rounds, 0, group, at(group.seats, 1))).toMatchObject({ through: false, out: true });
      expect(seatView(rounds, 0, group, at(group.seats, 0))).toMatchObject({ through: true, out: false });
    });

    it('gives its loser second place on their road', () => {
      expect(road(bracketRounds(bracket(match(0))), 2).map((s) => s.result)).toEqual(['2nd · 1 pts']);
    });
  });

  it('shows a group still being played without results', () => {
    const live = bracketRounds(inPlay);
    const group = at(at(live, 1).groups, 0);
    expect(seatView(live, 1, group, at(group.seats, 0))).toEqual({ through: false, out: false, to: '', points: '' });
  });
});

describe('road', () => {
  it("follows a player's groups to the end, with each result", () => {
    expect(road(bracketRounds(whole), 9).map((s) => [s.round, s.label, s.result])).toEqual([
      ['Quarter-finals', 'Group 3', '1st · 6 pts'],
      ['Semi-finals', 'Group 2', '1st · 6 pts'],
      ['Final', 'The final group', 'Champion · 6 pts'],
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

// A blitz knockout of four: its qualifiers by their place in the draw, then the semi-finals, the first decided.
const seeded = (entrant: number, name: string, seed: number, place = 0, next = 0): BracketSeat => ({ entrant, name, seed, place, next });
const blitz: SeasonBracket = {
  run_id: 1,
  slug: 'grand_blitz',
  season: '2026',
  rounds: [
    {
      stage: 'Qualifiers',
      qualifiers: true,
      groups: [
        played(1, [seeded(1, 'Aster', 1, 1, 1)]),
        played(2, [seeded(2, 'Brook', 4, 1, 1)]),
        played(3, [seeded(3, 'Cove', 2, 1, 2)]),
        played(4, [seeded(4, 'Dune', 3, 1, 2)]),
      ],
    },
    {
      stage: 'Round 2',
      qualifiers: false,
      groups: [played(1, [seeded(2, 'Brook', 4, 1), seeded(1, 'Aster', 1, 2)]), { name: 'match 2', number: 2, state: 'prepared', seats: [seeded(3, 'Cove', 2), seeded(4, 'Dune', 3)] }],
    },
  ],
};

describe('a blitz knockout', () => {
  const rounds = bracketRounds(blitz);

  it('opens with its qualifiers, then numbers its rounds from the first match', () => {
    expect(rounds.map((r) => [r.name, r.short, r.state])).toEqual([
      ['Qualifiers', 'Q', 'finished'],
      ['Semi-finals', 'SF', 'in_play'],
      ['Final', 'F', 'to_come'],
    ]);
    const big = bracketRounds({ ...blitz, rounds: [{ stage: 'Qualifiers', qualifiers: true, groups: Array.from({ length: 32 }, (_, i) => played(i + 1, [])) }] });
    expect(big.map((r) => r.short)).toEqual(['Q', 'R1', 'R2', 'QF', 'SF', 'F']);
  });

  it('is a tree from its qualifiers', () => {
    expect(treeReach(rounds)).toBe(0);
  });

  it.each([
    ['a qualifier by their seed', 0, 1, false, 'Seed 4'],
    ['a match', 1, 1, false, 'Match 2'],
    ['a match in the tree', 1, 0, true, 'SF · Match 1'],
    ['the final', 2, 0, true, 'Final'],
  ])('labels %s', (_name, r, g, titled, want) => {
    expect(groupLabel(rounds, r, at(at(rounds, r).groups, g), titled)).toBe(want);
  });

  it("follows a player's road from their seed", () => {
    expect(road(rounds, 1).map((s) => [s.round, s.label, s.result])).toEqual([
      ['Qualifiers', 'Seed 1', 'Seed 1'],
      ['Semi-finals', 'Match 1', '2nd'],
    ]);
  });

  it('shows a qualifier through to their match', () => {
    const group = at(at(rounds, 0).groups, 2);
    expect(seatView(rounds, 0, group, at(group.seats, 0))).toMatchObject({ through: true, out: false, to: '→ SF 2' });
  });
});

describe('labels elsewhere', () => {
  const rounds = bracketRounds(whole);
  it.each([
    ['a group', 0, 2, false, 'Group 3'],
    ['a group in the tree', 1, 1, true, 'SF · Group 2'],
  ])('labels %s of a bracket of groups', (_name, r, g, titled, want) => {
    expect(groupLabel(rounds, r, at(at(rounds, r).groups, g), titled)).toBe(want);
  });

  it.each([
    ['a seeded seat', seeded(1, 'Aster', 3), '(3)'],
    ['an unseeded seat', seat(1, 'Aster'), ''],
  ])('writes the seed of %s', (_name, s, want) => {
    expect(seedLabel(s)).toBe(want);
  });
});

describe('bracketCard', () => {
  const row = (edition: string, knockout: boolean): BlitzEditionRow => ({
    tournament: 'grand_blitz',
    name: 'Grand Blitz',
    edition,
    format: { swiss_rounds: 8, knockout_size: 8 },
    entrants: 20,
    round: 3,
    champion: null,
    knockout,
    bracket: true,
  });
  it.each([
    ['a tournament that is not blitz', 'wimbledon', [row('2026', false)], '/ratings/brackets/?t=wimbledon', 'View the bracket'],
    ['a blitz edition whose knockout is drawn', 'grand_blitz', [row('2026', true)], '/ratings/brackets/?t=grand_blitz', 'View the bracket'],
    ['a blitz edition in its Swiss rounds', 'grand_blitz', [row('2026', false), row('2025', true)], '/blitz/?t=grand_blitz&e=2026', 'Swiss rounds: see the standings'],
  ])('links %s', (_name, slug, editions, href, cta) => {
    expect(bracketCard(slug, editions)).toEqual({ href, cta });
  });
});

describe('fitZoom and stepZoom', () => {
  it.each([
    ['a tree that fits', 1300, 1160, 1],
    ['a tree twice the width', 580, 1160, 0.5],
    ['a tree too wide to shrink further', 200, 1160, 0.4],
    ['a box not measured yet', 0, 1160, 1],
  ])('fits %s', (_name, box, tree, want) => {
    expect(fitZoom(box, tree)).toBeCloseTo(want);
  });

  it.each([
    ['in from fit', 0.62, 1, 0.7],
    ['out from fit', 0.62, -1, 0.6],
    ['in at the largest', 1.5, 1, 1.5],
    ['out at the smallest', 0.4, -1, 0.4],
    ['in from full size', 1, 1, 1.25],
  ])('zooms %s', (_name, z, dir, want) => {
    expect(stepZoom(z, dir as 1 | -1)).toBe(want);
  });
});

describe('pinchZoom', () => {
  it.each([
    ['fingers spread to twice as far apart', 0.6, 100, 200, 1.2],
    ['fingers drawn to half as far apart', 1, 200, 100, 0.5],
    ['spread past the largest size', 1, 100, 400, 1.5],
    ['drawn past the smallest size', 0.5, 400, 100, 0.4],
    ['a pinch that starts with the fingers together', 0.8, 0, 100, 0.8],
  ])('sizes the tree for %s', (_name, from, start, now, want) => {
    expect(pinchZoom(from, start, now)).toBeCloseTo(want);
  });
});

describe('anchoredScroll', () => {
  it.each([
    ['doubling, the point 100 px into the tree', 0, 100, 1, 2, 100],
    ['halving, the point 300 px into the tree', 400, 300, 1, 0.5, 250],
    ['not changing size', 120, 300, 0.8, 0.8, 120],
    ['growing a tree scrolled to its left edge, the point at its edge', 0, 0, 1, 1.5, 0],
  ])('keeps a point still when %s', (_name, scroll, offset, from, to, want) => {
    expect(anchoredScroll(scroll, offset, from, to)).toBeCloseTo(want);
  });
});
