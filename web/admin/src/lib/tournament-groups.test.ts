import { describe, expect, it } from 'vitest';
import {
  candidatesFor,
  dateSpan,
  detail,
  groupChange,
  groupsAbove,
  groupsToJoin,
  groupTotals,
  isRated,
  listed,
  membersOf,
  moveChoices,
  newGroupBody,
  rowTotals,
  seasonDestinations,
  toggled,
  topLevel,
  typeLabel,
  unrelated,
} from './tournament-groups';

// The group worlds holds the group wc_2027, which holds wc_2027_tiebreak; swiss_open and slow_burn are related to
// nothing; leagues is an empty league group.
const LIST = [
  { slug: 'leagues', name: 'Leagues', parent: '', kind: 'league', is_group: true },
  { slug: 'quick_2p', name: 'Quick and Dirty 2p', parent: '', kind: 'league', is_group: false },
  { slug: 'slow_burn', name: 'Slow Burn', parent: '', kind: 'cup', is_group: false },
  { slug: 'swiss_open', name: 'Swiss Open 2022', parent: '', kind: 'cup', is_group: false },
  { slug: 'wc_2027', name: 'World Championship 2027', parent: 'worlds', kind: 'cup', is_group: true },
  { slug: 'wc_2027_tiebreak', name: 'WC 2027 tie-breaker', parent: 'wc_2027', kind: 'cup', is_group: false },
  { slug: 'worlds', name: 'World Championship', parent: '', kind: 'group', is_group: true },
];
const slugs = (list: { slug: string }[]) => list.map((t) => t.slug);

describe('tournament groups', () => {
  it('lists only what is in no group at the top', () => {
    expect(slugs(topLevel(LIST))).toEqual(['leagues', 'quick_2p', 'slow_burn', 'swiss_open', 'worlds']);
  });

  it.each([
    ['worlds', ['wc_2027']],
    ['wc_2027', ['wc_2027_tiebreak']],
    ['swiss_open', []],
  ])('gives the members of %s', (slug, want) => {
    expect(slugs(membersOf(LIST, slug))).toEqual(want);
  });

  it('finds what is related to nothing: in no group and holding none', () => {
    expect(slugs(unrelated(LIST))).toEqual(['leagues', 'quick_2p', 'slow_burn', 'swiss_open']);
  });

  it.each([
    ['wc_2027_tiebreak', ['worlds', 'wc_2027']],
    ['wc_2027', ['worlds']],
    ['worlds', []],
    ['no_such', []],
  ])('gives the groups above %s, outermost first', (slug, want) => {
    expect(slugs(groupsAbove(LIST, slug))).toEqual(want);
  });

  it.each([
    ['swiss_open', ['leagues', 'wc_2027', 'worlds']],
    ['worlds', ['leagues']],
    ['wc_2027_tiebreak', ['leagues', 'worlds']],
  ])('offers %s the groups it can join: not itself, its own group or one inside it', (slug, want) => {
    expect(slugs(groupsToJoin(LIST, slug))).toEqual(want);
  });

  it.each([
    ['a cup group offers cups in no group and its own members', 'wc_2027', false, ['slow_burn', 'swiss_open', 'wc_2027_tiebreak']],
    ['every type offers the rest too, never a group above it', 'wc_2027', true, ['leagues', 'quick_2p', 'slow_burn', 'swiss_open', 'wc_2027_tiebreak']],
    ['a league group offers leagues', 'leagues', false, ['quick_2p']],
    ['a group of groups offers any type, groups too', 'worlds', false, ['leagues', 'quick_2p', 'slow_burn', 'swiss_open', 'wc_2027']],
  ])('%s', (_, group, everyType, want) => {
    expect(slugs(candidatesFor(LIST, group, everyType))).toEqual(want);
  });

  it('a series offers any type, groups too, as a group of groups does', () => {
    const list = LIST.map((t) => (t.slug === 'worlds' ? { ...t, kind: 'series' } : t));
    expect(slugs(candidatesFor(list, 'worlds', false))).toEqual(['leagues', 'quick_2p', 'slow_burn', 'swiss_open', 'wc_2027']);
  });

  const GAMES = [
    { slug: 'series', parent: '', is_group: true, games: 0, first_game: null, last_game: null },
    { slug: 'edition_1', parent: 'series', is_group: false, games: 40, first_game: '2023-01-25', last_game: '2023-03-01' },
    { slug: 'finals', parent: 'series', is_group: true, games: 0, first_game: null, last_game: null },
    { slug: 'final_a', parent: 'finals', is_group: false, games: 3, first_game: '2024-02-17', last_game: '2024-02-18' },
    { slug: 'empty', parent: '', is_group: true, games: 0, first_game: null, last_game: null },
  ];
  it.each([
    ['series', { games: 43, first: '2023-01-25', last: '2024-02-18' }],
    ['finals', { games: 3, first: '2024-02-17', last: '2024-02-18' }],
    ['empty', { games: 0, first: null, last: null }],
  ])('adds up the games and dates of everything in %s, at any depth', (slug, want) => {
    expect(groupTotals(GAMES, slug)).toEqual(want);
  });

  it.each([
    ['2023-02-26', '2023-03-29', '2023-02-26 to 2023-03-29'],
    ['2023-02-26', '2023-02-26', '2023-02-26'],
    [null, null, 'no games'],
  ])('writes the dates of games from %s to %s', (first, last, want) => {
    expect(dateSpan(first, last)).toBe(want);
  });

  it.each([
    ['admin\'s description first', { description: 'Ours', cge_about: 'CGE\'s' }, 'Ours'],
    ['else CGE\'s, on one line', { description: '', cge_about: 'Rules:\n\nplay  well' }, 'Rules: play well'],
    ['cut at 140 characters', { description: 'x'.repeat(150), cge_about: '' }, `${'x'.repeat(139)}…`],
  ])('describes a row: %s', (_, t, want) => {
    expect(detail(t)).toBe(want);
  });

  const RATED = [
    { slug: 'mixed', parent: '', is_group: true, rated: false },
    { slug: 'a', parent: 'mixed', is_group: false, rated: false },
    { slug: 'inner', parent: 'mixed', is_group: true, rated: false },
    { slug: 'b', parent: 'inner', is_group: false, rated: true },
    { slug: 'unrated', parent: '', is_group: true, rated: false },
    { slug: 'c', parent: 'unrated', is_group: false, rated: false },
    { slug: 'lone', parent: '', is_group: false, rated: true },
  ];
  it.each([
    ['a group holding a rated tournament at any depth', 'mixed', true],
    ['a group holding none', 'unrated', false],
    ['a tournament, by its own setting', 'lone', true],
    ['an unrated tournament', 'a', false],
  ])('counts as rated: %s', (_, slug, want) => {
    expect(isRated(RATED, slug)).toBe(want);
  });

  it.each([
    ['a tournament by its kind', { kind: 'cup', is_group: false }, 'cup'],
    ['a series group', { kind: 'series', is_group: true }, 'series'],
    ['a group of groups', { kind: 'group', is_group: true }, 'group of groups'],
    ['a group of one kind', { kind: 'league', is_group: true }, 'group of leagues'],
  ])('labels the type of %s', (_, t, want) => {
    expect(typeLabel(t)).toBe(want);
  });

  const ROWS = [
    { slug: 'g', parent: '', is_group: true, games: 0, first_game: null, last_game: null, rated: false },
    { slug: 'a', parent: 'g', is_group: false, games: 5, first_game: '2024-01-01', last_game: '2024-02-01', rated: true },
    { slug: 'b', parent: '', is_group: false, games: 2, first_game: '2023-01-01', last_game: '2023-01-05', rated: false },
  ];
  it.each([
    ['a group by what is in it', ROWS[0], { games: 5, first: '2024-01-01', last: '2024-02-01' }],
    ['a tournament by its own games', ROWS[2], { games: 2, first: '2023-01-01', last: '2023-01-05' }],
  ])('totals %s', (_, row, want) => {
    expect(rowTotals(ROWS, row)).toEqual(want);
  });

  it.each([
    [true, ['g']],
    [false, ['b']],
  ])('lists the top level, rated %s', (rated, want) => {
    expect(slugs(listed(ROWS, rated))).toEqual(want);
  });

  it.each([
    ['a tournament in a group: no group first, then the others', 'wc_2027_tiebreak', [['', 'No group: the top level'], ['leagues', 'Leagues'], ['worlds', 'World Championship']]],
    ['one in no group: the groups only', 'swiss_open', [['leagues', 'Leagues'], ['wc_2027', 'World Championship 2027'], ['worlds', 'World Championship']]],
    ['a group with nowhere to go', 'worlds', [['leagues', 'Leagues']]],
  ])('offers moving %s', (_, slug, want) => {
    expect(moveChoices(LIST, slug).map((c) => [c.parent, c.label])).toEqual(want);
  });

  it.each([
    ['adds one not chosen', ['a'], 'b', ['a', 'b']],
    ['removes one chosen', ['a', 'b'], 'a', ['b']],
  ])('toggles a choice: %s', (_, chosen, slug, want) => {
    expect(toggled(chosen, slug)).toEqual(want);
  });

  it('finds destinations for moving a season excluding groups and self', () => {
    expect(slugs(seasonDestinations(LIST, 'slow_burn'))).toEqual(['quick_2p', 'swiss_open', 'wc_2027_tiebreak']);
  });

  const G = { name: 'Swiss Open', kind: 'series', description: 'Every year' };
  it.each([
    ['nothing changed', { name: ' Swiss Open ', kind: 'series', description: 'Every year ' }, null],
    ['an empty name', { name: '  ', kind: 'series', description: 'Every year' }, null],
    ['a new name and description, trimmed', { name: ' Swiss Opens ', kind: 'series', description: ' Annual ' }, { name: 'Swiss Opens', kind: 'series', description: 'Annual' }],
    ['a new type', { name: 'Swiss Open', kind: 'cup', description: 'Every year' }, { name: 'Swiss Open', kind: 'cup', description: 'Every year' }],
  ])('saves a group: %s', (_, form, want) => {
    expect(groupChange(G, form)).toEqual(want);
  });

  it.each([
    ['a name, trimmed', ' Quick and Dirty ', 'series', { name: 'Quick and Dirty', kind: 'series' }],
    ['no name', '   ', 'cup', null],
  ])('makes a new group from %s', (_, name, kind, want) => {
    expect(newGroupBody(name, kind)).toEqual(want);
  });
});
