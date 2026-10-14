import { describe, expect, it } from 'vitest';
import { byeTakers, canDeleteRound, entrantButtons, partnersFor, leftUnpaired, pairCounts, reconcilePending, replaceableSide, canPair, decidedOnly, nextRound, openRound, pairAction, rulingChoices, rulingLabel, newestFirst, samePairs, shownAfter, undecided, unpaired, unsavedMatch, zoneGapLabel, type AdminBlitzRound } from './blitz-rounds';

const round = (id: number, number: number, published: boolean, matches: AdminBlitzRound['matches']): AdminBlitzRound => ({
  id,
  number,
  kind: 'swiss',
  timezone: true,
  rating: false,
  published_at: published ? '2026-10-12T12:00:00Z' : null,
  matches,
  reserve: null,
});
const knockout = (id: number, number: number, published: boolean, matches: AdminBlitzRound['matches']): AdminBlitzRound => ({ ...round(id, number, published, matches), kind: 'knockout' });
const match = (id: number, a: number, b: number | null, result: 'a' | 'b' | 'draw' | 'double_loss' | null = null): AdminBlitzRound['matches'][number] => ({
  id,
  a,
  b,
  slot: null,
  a_seed: null,
  b_seed: null,
  zone_gap_minutes: 0,
  rating_gap: null,
  repeat: false,
  result,
  game: null,
  ruling: result === null ? '' : 'ruled',
  decided_at: result === null ? null : '2026-10-15T09:00:00Z',
});
const entrant = (id: number, status: 'active' | 'dropped' = 'active') => ({
  id,
  member: 1000 + id,
  name: `p${id}`,
  player: '',
  discord_name: '',
  email: '',
  country: null,
  zone: 'UTC',
  utc_offset: '+00:00',
  rating: null,
  status,
});

describe('canPair', () => {
  it.each([
    ['two active and no draft', [entrant(1), entrant(2)], [round(1, 1, true, [])], true],
    ['a draft open', [entrant(1), entrant(2)], [round(1, 1, false, [])], false],
    ['one active', [entrant(1), entrant(2, 'dropped')], [], false],
    ['nobody', [], [], false],
    ['every match decided or a bye', [entrant(1), entrant(2), entrant(3)], [round(1, 1, true, [match(1, 1, 2, 'a'), match(2, 3, null)])], true],
    ['a match without a result', [entrant(1), entrant(2)], [round(1, 1, true, [match(1, 1, 2)])], false],
  ])('says %s', (_name, entrants, rounds, want) => {
    expect(canPair(entrants, rounds)).toBe(want);
  });
});

describe('unsavedMatch and samePairs', () => {
  it('makes a match with no id, no gaps and no result', () => {
    expect(unsavedMatch(1, 2)).toEqual({ id: 0, a: 1, b: 2, slot: null, a_seed: null, b_seed: null, zone_gap_minutes: 0, rating_gap: null, repeat: false, result: null, game: null, ruling: '', decided_at: null });
    expect(unsavedMatch(3, null).b).toBe(null);
  });
  it.each([
    ['the same pairs in the same order', [match(1, 1, 2), match(2, 3, null)], [unsavedMatch(1, 2), unsavedMatch(3, null)], true],
    ['the same pairs in another order', [match(1, 1, 2), match(2, 3, null)], [unsavedMatch(3, null), unsavedMatch(1, 2)], true],
    ['a different pair', [match(1, 1, 2), match(2, 3, 4)], [unsavedMatch(1, 3), unsavedMatch(2, 4)], false],
    ['a pair turned into a bye', [match(1, 1, 2)], [unsavedMatch(1, null)], false],
    ['one match fewer', [match(1, 1, 2), match(2, 3, 4)], [unsavedMatch(1, 2)], false],
    ['the same two players either way round', [match(1, 1, 2)], [unsavedMatch(2, 1)], true],
  ])('sees %s', (_name, saved, pending, want) => {
    expect(samePairs(saved, pending)).toBe(want);
  });
});

describe('shownAfter', () => {
  const saved = [match(1, 1, 2), match(2, 3, 4), match(3, 5, null)];
  it.each([
    ['the saved round again for the same pairs, as Pair proposes them after an unpair', [unsavedMatch(1, 2), unsavedMatch(4, 3), unsavedMatch(5, null)], null],
    ['each pair kept as saved, the new ones unsaved', [unsavedMatch(2, 1), unsavedMatch(3, 5), unsavedMatch(4, null)], [1, 0, 0]],
    ['what is left after an unpair', [match(1, 1, 2), match(3, 5, null)], [1, 3]],
  ])('shows %s', (_name, changed, want) => {
    const got = shownAfter(saved, changed);
    expect(got === null ? null : got.map((m) => m.id)).toEqual(want);
  });
});

describe('canDeleteRound', () => {
  it.each([
    ['a draft', round(1, 1, false, [match(1, 1, 2)]), true],
    ['a published round with nothing played', round(1, 1, true, [match(1, 1, 2), match(2, 3, null)]), true],
    ['a published round with a result', round(1, 1, true, [match(1, 1, 2, 'a'), match(2, 3, 4)]), false],
  ])('says whether %s can be deleted', (_name, r, want) => {
    expect(canDeleteRound(r)).toBe(want);
  });
});

describe('undecided', () => {
  it('names the matches still without a result, byes aside, oldest round first', () => {
    const rounds = [round(2, 2, true, [match(3, 1, 3), match(4, 2, null)]), round(1, 1, true, [match(1, 1, 2, 'draw'), match(2, 3, 4)])];
    expect(undecided(rounds, (id) => `p${id}`)).toEqual(['p3 v p4', 'p1 v p3']);
  });
  it('names a knockout match without a winner, a draw or a double loss', () => {
    const rounds = [round(1, 1, true, [match(1, 1, 2, 'a')]), knockout(2, 2, true, [match(2, 1, 3, 'draw'), match(3, 2, 4, 'b'), match(4, 5, 6, 'double_loss')])];
    expect(undecided(rounds, (id) => `p${id}`)).toEqual(['p1 v p3 (no winner yet)', 'p5 v p6 (no winner yet)']);
  });
});

describe('nextRound', () => {
  const usOpen = { swiss_rounds: 2, knockout_size: 4 };
  it.each([
    ['round 1 before anything', [], usOpen, null, { number: 1, kind: 'swiss' }],
    ['a Swiss round while some are left', [round(1, 1, true, [])], usOpen, null, { number: 2, kind: 'swiss' }],
    ['the knockout after the last Swiss round', [round(1, 1, true, []), round(2, 2, true, [])], usOpen, null, { number: 3, kind: 'knockout' }],
    ['the knockout again after a knockout round', [round(1, 1, true, []), round(2, 2, true, []), knockout(3, 3, true, [])], usOpen, null, { number: 4, kind: 'knockout' }],
    ['nothing once there is a champion', [round(1, 1, true, []), round(2, 2, true, []), knockout(3, 3, true, []), knockout(4, 4, true, [])], usOpen, 'p1', null],
    ['nothing after the Swiss with no knockout', [round(1, 1, true, []), round(2, 2, true, [])], { swiss_rounds: 2, knockout_size: 0 }, null, null],
  ])('pairs %s', (_name, rounds, format, champion, want) => {
    expect(nextRound(rounds, format, champion)).toEqual(want);
  });
});

describe('rulingChoices', () => {
  it.each([
    ['a Swiss match', 'swiss', ['a', 'b', 'draw', 'double_loss']],
    ['a knockout match, which needs a winner', 'knockout', ['a', 'b']],
  ] as const)('offers %s', (_name, kind, want) => {
    expect(rulingChoices(kind)).toEqual(want);
  });
});

describe('rulingLabel', () => {
  it.each([
    ['a win with no game, a forfeit by the other', 'a', false, 'Cy forfeits: Ann wins'],
    ['the other win with no game', 'b', false, 'Ann forfeits: Cy wins'],
    ['a draw with no game', 'draw', false, 'Draw: neither at fault'],
    ['a double loss with no game', 'double_loss', false, 'Both lose: both at fault'],
    ['a win overruling a game', 'a', true, 'Ann wins'],
    ['a draw overruling a game', 'draw', true, 'A draw'],
    ['a double loss overruling a game', 'double_loss', true, 'Both lose'],
  ] as const)('names %s', (_name, result, played, want) => {
    expect(rulingLabel(result, 'Ann', 'Cy', played)).toBe(want);
  });
});

describe('newestFirst', () => {
  it('orders rounds newest first', () => {
    const rounds = [round(1, 1, true, []), round(2, 2, true, []), round(3, 3, false, [])];
    expect(newestFirst(rounds).map((r) => r.number)).toEqual([3, 2, 1]);
  });
});

describe('openRound', () => {
  const done = round(1, 1, true, [match(1, 1, 2, 'a'), match(2, 3, null)]);
  it.each([
    ['no round', [], null],
    ['a draft after a finished round', [done, round(2, 2, false, [match(3, 1, 3)])], 2],
    ['a published round with a match to play', [done, round(2, 2, true, [match(3, 1, 3, 'a'), match(4, 2, 4)])], 2],
    ['a finished round, byes aside', [done], null],
    ['a knockout round with a drawn match', [done, knockout(2, 2, true, [match(3, 1, 2, 'draw')])], 2],
    ['a finished knockout round', [done, knockout(2, 2, true, [match(3, 1, 2, 'b')])], null],
  ])('finds %s', (_name, rounds, want) => {
    expect(openRound(rounds)?.id ?? null).toBe(want);
  });
});

describe('pairAction', () => {
  const format = { swiss_rounds: 2, knockout_size: 2 };
  const four = [entrant(1), entrant(2), entrant(3), entrant(4)];
  const done = round(1, 1, true, [match(1, 1, 2, 'a'), match(2, 3, 4, 'b')]);
  it.each([
    ['the first round', four, [], null, null, { kind: 'next', round: { number: 1, kind: 'swiss' } }],
    ['the next round once every result is in', four, [done], null, null, { kind: 'next', round: { number: 2, kind: 'swiss' } }],
    ['the knockout after the Swiss rounds', four, [done, round(2, 2, true, [match(3, 1, 3, 'a'), match(4, 2, 4, 'a')])], null, null, { kind: 'next', round: { number: 3, kind: 'knockout' } }],
    ['the next round, not a late entrant, once every result is in', [...four, entrant(5)], [done], null, null, { kind: 'next', round: { number: 2, kind: 'swiss' } }],
    ['the players a draft leaves out', four, [round(1, 1, false, [match(1, 1, 2)])], null, null, { kind: 'fill', round: 1 }],
    ['the players unpaired by hand', four, [round(1, 1, false, [match(1, 1, 2), match(2, 3, 4)])], [unsavedMatch(1, 2)], null, { kind: 'fill', round: 1 }],
    ['a late entrant while a round is played', [...four, entrant(5)], [round(1, 1, true, [match(1, 1, 2, 'a'), match(2, 3, 4)])], null, null, { kind: 'fill', round: 1 }],
    ['nothing while a draft pairs everyone', four, [round(1, 1, false, [match(1, 1, 2), match(2, 3, 4)])], null, null, { kind: 'none' }],
    ['nothing while a knockout round is played', four, [done, round(2, 2, true, []), knockout(3, 3, true, [match(3, 1, 2)])], null, null, { kind: 'none' }],
    ['nothing once there is a champion', four, [done, round(2, 2, true, []), knockout(3, 3, true, [match(3, 1, 2, 'a')])], null, 'p1', { kind: 'none' }],
    ['nothing with one active entrant', [entrant(1), entrant(2, 'dropped')], [], null, null, { kind: 'none' }],
  ])('pairs %s', (_name, entrants, rounds, shown, champion, want) => {
    expect(pairAction({ entrants, rounds, format, champion }, shown)).toEqual(want);
  });
});

describe('unpaired', () => {
  it('lists the active entrants the round leaves out', () => {
    const entrants = [entrant(1), entrant(2), entrant(3), entrant(4, 'dropped'), entrant(5)];
    expect(unpaired(entrants, round(9, 1, false, [match(1, 1, 2), match(2, 5, null)])).map((e) => e.id)).toEqual([3]);
  });
});

describe('zoneGapLabel', () => {
  it.each([
    [0, 'same hour'],
    [60, '1 h apart'],
    [90, '1 h 30 apart'],
    [360, '6 h apart'],
  ])('shows %d minutes as %s', (minutes, want) => {
    expect(zoneGapLabel(minutes)).toBe(want);
  });
});

describe('decidedOnly', () => {
  it.each([
    ['a fresh draft', [match(1, 1, 2), match(2, 3, 4), match(3, 5, null)], []],
    ['a round with one result', [match(1, 1, 2, 'a'), match(2, 3, 4), match(3, 5, null)], [1]],
    ['a round with every result', [match(1, 1, 2, 'draw'), match(2, 3, 4, 'b')], [1, 2]],
    ['no matches', [], []],
  ])('keeps only the matches with a result in %s', (_name, matches, want) => {
    expect(decidedOnly(matches).map((m) => m.id)).toEqual(want);
  });
});

describe('entrantButtons', () => {
  const rounds = [
    round(1, 1, true, [match(1, 1, 2, 'a'), match(2, 3, 4), match(3, 5, null)]),
    round(2, 2, false, [match(4, 6, null)]),
  ];
  it.each([
    ['Drop for one who played', entrant(1), { confirm: false, drop: 'drop', remove: null }],
    ['Reinstate for one who dropped', entrant(2, 'dropped'), { confirm: false, drop: 'reinstate', remove: null }],
    ['Remove for one paired who has not played', entrant(3), { confirm: false, drop: null, remove: 'remove' }],
    ['Drop for a bye in a published round', entrant(5), { confirm: false, drop: 'drop', remove: null }],
    ['Remove for a bye in a draft', entrant(6), { confirm: false, drop: null, remove: 'remove' }],
    ['Confirm and Remove for one not confirmed', { ...entrant(7), status: 'pending' as const }, { confirm: true, drop: null, remove: 'remove' }],
    ['Reinstate and Remove for one who dropped before playing', entrant(8, 'dropped'), { confirm: false, drop: 'reinstate', remove: 'remove' }],
  ])('%s', (_name, e, want) => {
    expect(entrantButtons(e, rounds)).toEqual(want);
  });
});

describe('replaceableSide', () => {
  const people = [entrant(1, 'dropped'), entrant(2), entrant(3), entrant(4, 'dropped')];
  it.each([
    ['side a, who dropped, in a knockout match to play', knockout(1, 2, true, [match(1, 1, 2)]), 1],
    ['side b, who dropped', knockout(1, 2, true, [match(1, 3, 4)]), 4],
    ['nobody when neither dropped', knockout(1, 2, true, [match(1, 2, 3)]), null],
    ['nobody in a Swiss round', round(1, 1, true, [match(1, 1, 2)]), null],
    ['nobody once the match has a result', knockout(1, 2, true, [match(1, 1, 2, 'b')]), null],
  ])('%s', (_name, r, want) => {
    expect(replaceableSide(r.matches[0], r, people)).toBe(want);
  });
});

describe('leftUnpaired', () => {
  const rounds = [round(1, 1, true, [match(1, 1, 2, 'a'), match(2, 3, 4), match(3, 5, null)]), round(2, 2, false, [match(4, 1, 3)])];
  it.each([
    ['the opponents of their matches still to play', 3, [4, 1]],
    ['nobody for a bye', 5, []],
    ['nobody for a match with a result', 2, []],
  ])('names %s', (_name, id, want) => {
    expect(leftUnpaired(id, rounds)).toEqual(want);
  });
});

describe('reconcilePending', () => {
  const saved = round(1, 1, true, [match(1, 1, 2), match(2, 3, 4), match(3, 5, null)]);
  const everyone = [entrant(1), entrant(2), entrant(3), entrant(4), entrant(5)];
  const without2 = everyone.filter((e) => e.id !== 2);
  const unpairedOne = { round: 1, matches: [match(2, 3, 4), match(3, 5, null)] };
  it.each([
    ['nothing for nothing unsaved', null, [saved], everyone, null],
    ['nothing once its round is gone', unpairedOne, [], everyone, null],
    ['the changes as they were while everyone is still entered', unpairedOne, [saved], everyone, [2, 3]],
    ['the changes without the matches of someone removed', { round: 1, matches: [unsavedMatch(2, 1), match(2, 3, 4), match(3, 5, null)] }, [saved], without2, [2, 3]],
  ])('keeps %s', (_name, pending, rounds, entrants, want) => {
    const got = reconcilePending(pending, rounds, entrants);
    expect(got === null ? null : got.matches.map((m) => m.id)).toEqual(want);
  });

  it('shows the saved round when what is left pairs as saved', () => {
    const afterRemoval = round(1, 1, true, [match(2, 3, 4), match(3, 5, null)]);
    expect(reconcilePending({ round: 1, matches: [unsavedMatch(1, 2), match(2, 3, 4), match(3, 5, null)] }, [afterRemoval], without2)).toBe(null);
  });
});

describe('pairCounts', () => {
  const entrants = [entrant(1), entrant(2), entrant(3), entrant(4), entrant(5, 'dropped'), { ...entrant(6), status: 'pending' as const }, entrant(7)];
  it.each([
    ['the players in a match or with the bye, and the active ones left out', [match(1, 1, 2), match(2, 3, null)], { paired: 3, unpaired: 2 }],
    ['nobody paired', [], { paired: 0, unpaired: 5 }],
    ['everyone paired', [match(1, 1, 2), match(2, 3, 4), match(3, 7, null)], { paired: 5, unpaired: 0 }],
  ])('counts %s', (_name, matches, want) => {
    expect(pairCounts(entrants, matches)).toEqual(want);
  });
});

describe('partnersFor and byeTakers', () => {
  const free = [entrant(1), entrant(2), entrant(3), entrant(4)];
  const rounds = [
    round(1, 1, true, [match(1, 1, 2, 'a'), match(2, 3, null), match(3, 4, 5, 'b')]),
    knockout(2, 2, true, [match(4, 1, 3)]),
    round(3, 3, false, [match(5, 1, 4)]),
  ];
  it.each([
    ['everyone unpaired for nobody picked', null, [1, 2, 3, 4]],
    ['not themself, nor anyone met in another Swiss round', 1, [3, 4]],
    ['a knockout meeting is no bar', 3, [1, 2, 4]],
  ])('partners: %s', (_name, id, want) => {
    expect(partnersFor(id, free, rounds, 3).map((e) => e.id)).toEqual(want);
  });
  it('a pairing in the round being changed is no bar', () => {
    expect(partnersFor(4, free, rounds, 3).map((e) => e.id)).toEqual([1, 2, 3]);
  });
  it.each([
    ['the unpaired who have not had a bye', free, [1, 2, 4]],
    ['everyone when all have had one', [entrant(3)], [3]],
  ])('bye: %s', (_name, players, want) => {
    expect(byeTakers(players, rounds, 3).map((e) => e.id)).toEqual(want);
  });
});
