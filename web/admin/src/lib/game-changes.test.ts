import { describe, expect, it } from 'vitest';
import {
  changeBody,
  changeRows,
  emptyEntry,
  entryFrom,
  foundLine,
  groupChoices,
  undoText,
  withoutSeat,
  withSeat,
  type Action,
  type ChangeableGame,
  type EntryForm,
  type GameChange,
  type Picked,
} from './game-changes';

const original: ChangeableGame = {
  id: 3, name: 'Intermezzo S24 - Bronze 1 game 1', code: 'AAAAAAAA', date: '2024-09-16', state: 'played', group: 5, number: 1,
  seats: [
    { player: 'Player One', place: 1, outcome: 'finished', score: 120 },
    { player: 'Player Two', place: 2, outcome: null, score: 0 },
  ],
};

const filled: EntryForm = {
  code: ' NEWGAME1 ', date: '2024-09-20', number: '',
  seats: [
    { player: ' Player One ', place: '1', outcome: 'finished', culture: '120.5' },
    { player: 'Player Two', place: '2', outcome: 'timed_out', culture: '40' },
  ],
};

describe('game changes', () => {
  it("starts a replacement from the original's players and day, with no code", () => {
    const e = entryFrom(original);
    expect(e.code).toBe('');
    expect(e.date).toBe('2024-09-16');
    expect(e.seats).toEqual([
      { player: 'Player One', place: '1', outcome: 'finished', culture: '120' },
      { player: 'Player Two', place: '2', outcome: 'finished', culture: '0' },
    ]);
  });

  it('adds seats up to four, each at the next place, and takes them away down to two', () => {
    const four = withSeat(withSeat(emptyEntry()));
    expect(four.seats.map((s) => s.place)).toEqual(['1', '2', '3', '4']);
    expect(withSeat(four).seats).toHaveLength(4);
    const three = withoutSeat(four, 1);
    expect(three.seats.map((s) => s.place)).toEqual(['1', '3', '4']);
    expect(withoutSeat(withoutSeat(three, 0), 0).seats).toHaveLength(2);
  });

  it.each([
    ['an addition', 'add', { group: 5 }, { action: 'add', group: 5, reason: 'Played outside the tournament' }],
    ['a removal', 'remove', { game: 3 }, { action: 'remove', game: 3, reason: 'Played outside the tournament' }],
    ['a replacement', 'replace', { game: 3 }, { action: 'replace', game: 3, reason: 'Played outside the tournament' }],
  ] as const)('sends %s with what its action takes', (_name, action, picked, want) => {
    const r = changeBody(action, { ...picked, entry: filled, reason: ' Played outside the tournament ' });
    expect(r.ok && r.body).toMatchObject(want);
    expect(r.ok && 'entry' in r.body).toBe(action !== 'remove');
  });

  it('sends the entry trimmed, culture as numbers, 0 for a player who did not finish, and a game number only when given', () => {
    const r = changeBody('add', { group: 5, entry: filled, reason: 'A reason' });
    expect(r.ok && r.body.entry).toEqual({
      code: 'NEWGAME1', date: '2024-09-20',
      seats: [
        { player: 'Player One', place: 1, outcome: 'finished', score: 120.5 },
        { player: 'Player Two', place: 2, outcome: 'timed_out', score: 0 },
      ],
    });
    const numbered = changeBody('add', { group: 5, entry: { ...filled, number: '7' }, reason: 'A reason' });
    expect(numbered.ok && numbered.body.entry?.number).toBe(7);
  });

  const missing: [string, Action, Partial<Picked>, string][] = [
    ['no reason', 'remove', { game: 3, reason: ' ' }, 'Say why'],
    ['no game found', 'replace', { reason: 'A reason' }, 'Find the game'],
    ['no group chosen', 'add', { reason: 'A reason' }, 'Choose the group'],
    ['culture that is no number', 'add', { group: 5, reason: 'A reason', entry: { ...filled, seats: [{ ...filled.seats[0], culture: 'lots' }, filled.seats[1]] } }, 'culture'],
    ['no culture for a player who finished', 'add', { group: 5, reason: 'A reason', entry: { ...filled, seats: [{ ...filled.seats[0], culture: '' }, filled.seats[1]] } }, 'culture'],
    ['a game number that is no whole number', 'add', { group: 5, reason: 'A reason', entry: { ...filled, number: '2.5' } }, 'game number'],
  ];
  it.each(missing)('says what is missing: %s', (_name, action, given, want) => {
    const r = changeBody(action, { entry: filled, reason: '', ...given });
    expect(r.ok).toBe(false);
    expect(!r.ok && r.message).toContain(want);
  });

  it('lists each change with its games, by name and code, and who made it', () => {
    const changes: GameChange[] = [
      {
        id: 7, action: 'replace', reason: 'Timed out', at: '2026-11-20T01:00:00Z', by: 'An Organiser',
        game: { id: 3, name: 'Bronze 1 game 1', code: 'AAAAAAAA', state: 'replaced', date: '2024-09-16' },
        added: { id: 9, name: 'Bronze 1 game 1', code: '', state: 'played', date: '2024-09-20' },
      },
      { id: 6, action: 'add', reason: 'Missing', at: '2026-11-19T01:00:00Z', by: '', game: null, added: { id: 8, name: 'Bronze 1 game 3', code: 'CCCCCCCC', state: 'played', date: '2024-09-18' } },
    ];
    const rows = changeRows(changes, (iso) => iso.slice(0, 10));
    expect(rows.map((r) => [r.action, r.game, r.added, r.by, r.when])).toEqual([
      ['Replace', 'Bronze 1 game 1 (AAAAAAAA)', 'Bronze 1 game 1 (no code)', 'An Organiser', '2026-11-20'],
      ['Add', '', 'Bronze 1 game 3 (CCCCCCCC)', 'not known', '2026-11-19'],
    ]);
  });

  it.each([
    ['add', 'deleted'],
    ['remove', 'earlier state'],
    ['replace', 'deleted'],
  ] as const)('says what undoing %s does', (action, want) => {
    expect(undoText(action)).toContain(want);
  });

  it("names a season's groups by stage and group", () => {
    expect(groupChoices([
      { label: '', groups: [{ id: 5, name: 'Bronze 1', games: 2, next_number: 3 }] },
      { label: 'Final', groups: [{ id: 6, name: '', games: 0, next_number: 1 }] },
    ])).toEqual([
      { id: 5, label: 'Bronze 1 (2 games)', next: 3 },
      { id: 6, label: 'Final (0 games)', next: 1 },
    ]);
  });

  it('says what the game found is: its day, state and players in place order', () => {
    expect(foundLine(original)).toBe('2024-09-16, played: 1. Player One (finished, 120); 2. Player Two (not known, 0)');
    expect(foundLine({ ...original, state: 'never_played', seats: [] })).toBe('2024-09-16, never played: no players');
  });
});
