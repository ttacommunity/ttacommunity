import { describe, expect, it } from 'vitest';
import { canPair, editableRound, newestFirst, undecided, unpaired, withBye, withPair, withoutMatch, zoneGapLabel, type AdminBlitzRound } from './blitz-rounds';

const round = (id: number, number: number, published: boolean, matches: AdminBlitzRound['matches']): AdminBlitzRound => ({
  id,
  number,
  kind: 'swiss',
  timezone: true,
  rating: false,
  published_at: published ? '2026-10-12T12:00:00Z' : null,
  matches,
});
const match = (id: number, a: number, b: number | null, result: 'a' | 'b' | 'draw' | 'double_loss' | null = null): AdminBlitzRound['matches'][number] => ({
  id,
  a,
  b,
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
  name: `p${id}`,
  player: '',
  discord_name: '',
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

describe('undecided', () => {
  it('names the matches still without a result, byes aside, oldest round first', () => {
    const rounds = [round(2, 2, true, [match(3, 1, 3), match(4, 2, null)]), round(1, 1, true, [match(1, 1, 2, 'draw'), match(2, 3, 4)])];
    expect(undecided(rounds, (id) => `p${id}`)).toEqual(['p3 v p4', 'p1 v p3']);
  });
});

describe('newestFirst and editableRound', () => {
  const rounds = [round(1, 1, true, []), round(2, 2, true, []), round(3, 3, false, [])];
  it('orders rounds newest first', () => {
    expect(newestFirst(rounds).map((r) => r.number)).toEqual([3, 2, 1]);
  });
  it('lets only the latest round be changed', () => {
    expect(editableRound(rounds)).toBe(3);
    expect(editableRound([])).toBe(null);
  });
});

describe('unpaired', () => {
  it('lists the active entrants the round leaves out', () => {
    const entrants = [entrant(1), entrant(2), entrant(3), entrant(4, 'dropped'), entrant(5)];
    expect(unpaired(entrants, round(9, 1, false, [match(1, 1, 2), match(2, 5, null)])).map((e) => e.id)).toEqual([3]);
  });
});

describe('changing a round by hand', () => {
  const matches = [match(1, 1, 2), match(2, 3, null)];
  it('drops a match', () => {
    expect(withoutMatch(matches, 1)).toEqual([{ a: 3 }]);
  });
  it('adds a pair', () => {
    expect(withPair(matches, 4, 5)).toEqual([{ a: 1, b: 2 }, { a: 3 }, { a: 4, b: 5 }]);
  });
  it('gives the bye, taking it from whoever had it', () => {
    expect(withBye(matches, 6)).toEqual([{ a: 1, b: 2 }, { a: 6 }]);
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
