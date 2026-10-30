import { describe, expect, it } from 'vitest';
import {
  challengeHref,
  champions,
  challengePicture,
  challengePlayerHref,
  challengeSets,
  compareBests,
  compareHref,
  entryDay,
  byMeasure,
  matchingPlayers,
  nameParam,
  formatMeasure,
  groupLabel,
  leader,
  measureOf,
  niceTicks,
  placingSeparators,
  playedInSet,
  playNext,
  scopeLeaders,
  setLabel,
  signed,
} from './challenges';

const challenge = (id: string, set: 'original' | 'expansion' | 'mix', group: string) => ({ id, set, group });

describe('challengeSets', () => {
  it("puts the sets in the game's order, each group where its first challenge is, and the challenges as listed", () => {
    const list = [
      challenge('mix1', 'mix', 'mix'),
      challenge('a1', 'original', 'classic'),
      challenge('b1', 'original', 'epic'),
      challenge('a2', 'original', 'classic'),
      challenge('x1', 'expansion', 'gods'),
    ];
    const sets = challengeSets(list);
    expect(sets.map((s) => s.id)).toEqual(['original', 'expansion', 'mix']);
    expect(sets[0].groups.map((g) => [g.id, g.challenges.map((c) => c.id)])).toEqual([
      ['classic', ['a1', 'a2']],
      ['epic', ['b1']],
    ]);
  });

  it('leaves out a set with no challenges', () => {
    expect(challengeSets([challenge('a1', 'original', 'classic')]).map((s) => s.id)).toEqual(['original']);
  });
});

describe('groupLabel', () => {
  it.each([
    ['a group the game names', 'time_travellers', 'Time Travelers'],
    ['a group it does not know', 'new_group', 'New group'],
  ])('names %s', (_name, group, want) => {
    expect(groupLabel(group)).toBe(want);
  });
});

describe('challengePicture', () => {
  it("is the challenge's picture in the site's images", () => {
    expect(challengePicture('epic1_exp')).toBe('/img/challenges/epic1_exp.jpg');
  });
});

describe('the challenge links', () => {
  it.each([
    ['a challenge', challengeHref('epic1_exp'), '/challenges/epic1_exp/'],
    ['a player, by name', challengePlayerHref('Ann B&C'), '/challenges/player/?name=Ann+B%26C'],
    ['compare with one player', compareHref('Ann'), '/challenges/compare/?a=Ann'],
    ['compare two', compareHref('Ann', 'Bob'), '/challenges/compare/?a=Ann&b=Bob'],
  ])('leads to %s', (_name, got, want) => {
    expect(got).toBe(want);
  });
});

const summary = {
  player: 'Ann',
  played: 3,
  verified: 2,
  records: 1,
  total: 300,
  totals: { original: 100, expansion: 50, mix: 150 },
  rating: 42.34,
  ratings: { original: 10, expansion: 20, mix: 61.25 },
  last_entry: '2026-01-01T00:00:00Z',
};

describe('measureOf and formatMeasure', () => {
  it.each([
    ['the total', 'total', undefined, '300'],
    ["a set's total", 'total', 'mix', '150'],
    ['the rating, to a tenth', 'rating', undefined, '42.3%'],
    ["a set's rating", 'rating', 'mix', '61.3%'],
  ] as const)('shows %s', (_name, measure, set, want) => {
    expect(formatMeasure(measureOf(summary, measure, set), measure)).toBe(want);
  });
});

describe('scopeLeaders', () => {
  it('follows the chosen measure', () => {
    const scope = { by_total: [{ player: 'T', total: 1, played: 1, rating: 1 }], by_rating: [{ player: 'R', total: 1, played: 1, rating: 1 }] };
    expect(scopeLeaders(scope, 'total')[0].player).toBe('T');
    expect(scopeLeaders(scope, 'rating')[0].player).toBe('R');
  });
});

const best = (challenge: string, domination: number, record: number, share: number, rank = 2) => ({
  challenge,
  domination,
  proof_url: null,
  rank,
  entrants: 5,
  share,
  record,
  record_holder: 'Rec',
});

describe('playNext', () => {
  const bests = [best('a', 90, 100, 90), best('b', 10, 200, 5), best('c', 150, 500, 30), best('r', 300, 300, 100, 1)];
  it.each([
    ['by total: the fewest points to beat, and the lowest dominations', 'total', ['a', 'b', 'c'], ['b', 'a', 'c']],
    ['by rating: the highest share, and the lowest', 'rating', ['a', 'c', 'b'], ['b', 'c', 'a']],
  ] as const)('picks %s, leaving out records held', (_name, measure, closest, lowest) => {
    const got = playNext(bests, measure, 8);
    expect(got.closest.map((b) => b.challenge)).toEqual(closest);
    expect(got.lowest.map((b) => b.challenge)).toEqual(lowest);
  });
  it('shows at most as many as asked', () => {
    expect(playNext(bests, 'total', 2).closest).toHaveLength(2);
  });
});

describe('compareBests', () => {
  it('counts wins and ties on the challenges both played', () => {
    const a = [best('x', 10, 0, 0), best('y', 5, 0, 0), best('z', 7, 0, 0), best('only-a', 1, 0, 0)];
    const b = [best('x', 3, 0, 0), best('y', 9, 0, 0), best('z', 7, 0, 0), best('only-b', 1, 0, 0)];
    expect(compareBests(a, b)).toEqual({ winsA: 1, winsB: 1, ties: 1, both: 3 });
  });
});

describe('signed', () => {
  it.each([
    [5, '+5'],
    [-3, '-3'],
    [0, '±0'],
  ])('writes %d as %s', (value, want) => {
    expect(signed(value)).toBe(want);
  });
});

describe('niceTicks', () => {
  it.each([
    ['from 0 to past the top', 0, 230, [0, 100, 200, 300]],
    ['below 0 for a negative record', -40, 90, [-50, 0, 50, 100]],
    ['a flat line', 0, 0, [0]],
  ])('runs %s', (_name, min, max, want) => {
    expect(niceTicks(min, max)).toEqual(want);
  });
});

describe('entryDay', () => {
  it.each([
    ['in the zone given', '2025-02-24T23:30:00Z', 'UTC', '24 Feb 2025'],
    ['the next day where it already is', '2025-02-24T23:30:00Z', 'Europe/Berlin', '25 Feb 2025'],
  ])('writes the day %s', (_name, instant, zone, want) => {
    expect(entryDay(instant, zone)).toBe(want);
  });
});

describe('setLabel', () => {
  it('names a set as the game does', () => {
    expect(setLabel('mix')).toBe('Random Mix');
  });
});

describe('byMeasure', () => {
  it.each([
    ['total', ['T', 'R']],
    ['rating', ['R', 'T']],
  ] as const)('puts the best by %s first', (measure, want) => {
    const players = [
      { ...summary, player: 'T', total: 900, rating: 10 },
      { ...summary, player: 'R', total: 100, rating: 60 },
    ];
    expect(byMeasure(players, measure).map((p) => p.player)).toEqual(want);
  });
});

describe('matchingPlayers', () => {
  it.each([
    ['everyone for no search', '  ', ['Ann Lee', 'bob']],
    ['part of a name, ignoring case', 'LEE', ['Ann Lee']],
  ])('keeps %s', (_name, query, want) => {
    expect(matchingPlayers([{ player: 'Ann Lee' }, { player: 'bob' }], query).map((p) => p.player)).toEqual(want);
  });
});

describe('nameParam', () => {
  it.each([
    ['?name=Ann+B', 'Ann B'],
    ['?name=%20Ann%20', 'Ann'],
    ['', ''],
  ])('reads %s as %s', (search, want) => {
    expect(nameParam(search)).toBe(want);
  });
});

describe('champions', () => {
  it('counts wins, tied winners each getting one', () => {
    const weeks = [{ winners: [{ player: 'Bob' }] }, { winners: [{ player: 'Ann' }, { player: 'Bob' }] }, { winners: [{ player: 'Ann' }] }, { winners: [] }];
    expect(champions(weeks)).toEqual([
      ['Ann', 2],
      ['Bob', 2],
    ]);
  });
});

describe('leader', () => {
  it.each([
    ['a ahead', 10, 5, 'a'],
    ['b ahead', 5, 10, 'b'],
    ['a tie', 7, 7, ''],
    ['only a played', 7, undefined, 'a'],
    ['only b played', undefined, 7, 'b'],
    ['neither played', undefined, undefined, ''],
  ])('says who leads with %s', (_name, a, b, want) => {
    expect(leader(a, b)).toBe(want);
  });
});

describe('placingSeparators', () => {
  it.each([
    ['nothing before the first', [100], ['']],
    ['a comma between different dominations', [100, 90], ['', ', ']],
    ['an ampersand joining a tie', [100, 100, 90], ['', ' & ', ', ']],
  ])('writes %s', (_name, dominations, want) => {
    expect(placingSeparators(dominations.map((domination) => ({ domination })))).toEqual(want);
  });
});

describe('playedInSet', () => {
  const list = [challenge('a', 'original', 'g'), challenge('b', 'original', 'g'), challenge('c', 'mix', 'g')];
  it.each([
    ['the set with one of two played', 'original', { played: 1, of: 2 }],
    ['the set with its one challenge played', 'mix', { played: 1, of: 1 }],
    ['a set with no challenge', 'expansion', { played: 0, of: 0 }],
  ] as const)('counts %s', (_name, set, want) => {
    expect(playedInSet([{ challenge: 'a' }, { challenge: 'c' }, { challenge: 'gone' }], list, set)).toEqual(want);
  });
});
