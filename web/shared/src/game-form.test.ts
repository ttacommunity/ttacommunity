import { describe, expect, it } from 'vitest';
import { chooseTurn, emptyGameForm, gameRecordFrom, type GameForm } from './game-form';

const form: GameForm = {
  share_code: ' ABCD1234 ',
  a_culture: '250',
  a_outcome: 'finished',
  a_turn: 2,
  b_culture: '200',
  b_outcome: 'finished',
  b_turn: 1,
  finished_on: '2026-10-14',
  finished_time: '21:30',
  zone: 'Europe Central',
};

describe('gameRecordFrom', () => {
  it('turns the form into the request, trimmed and numbered', () => {
    expect(gameRecordFrom(form)).toEqual({
      ok: true,
      body: { share_code: 'ABCD1234', a: { culture: 250, outcome: 'finished', turn: 2 }, b: { culture: 200, outcome: 'finished', turn: 1 }, finished_on: '2026-10-14', finished_time: '21:30', zone: 'Europe Central' },
    });
  });
  it('refuses a game without a share code when one is required', () => {
    const r = gameRecordFrom({ ...form, share_code: ' ' }, { codeRequired: true });
    expect(!r.ok && r.message).toContain('share code');
    expect(gameRecordFrom(form, { codeRequired: true }).ok).toBe(true);
  });
  it('sends no share code for a game without one', () => {
    const r = gameRecordFrom({ ...form, share_code: ' ' });
    expect(r.ok && r.body.share_code).toBe('');
  });
  it('sends 0 for a player who resigned or timed out, whatever was typed', () => {
    const r = gameRecordFrom({ ...form, b_culture: '190', b_outcome: 'resigned' });
    expect(r.ok && r.body.b).toEqual({ culture: 0, outcome: 'resigned', turn: 1 });
  });
  it.each([
    ['a culture that is not a number', { ...form, a_culture: 'lots' }, 'culture'],
    ['a negative culture', { ...form, b_culture: '-1' }, 'culture'],
    ['an empty culture for a finished player', { ...form, a_culture: '' }, 'culture'],
    ['no day it finished', { ...form, finished_on: '' }, 'day'],
    ['no time it finished', { ...form, finished_time: '' }, 'time it finished'],
    ['no time zone', { ...form, zone: '' }, 'time zone'],
    ['nobody finished', { ...form, a_outcome: 'resigned', b_outcome: 'timed_out' }, 'finished'],
    ['no turn order', { ...form, a_turn: null, b_turn: null }, 'turn'],
    ["one player's turn missing", { ...form, b_turn: null }, 'turn'],
    ['both P1', { ...form, a_turn: 1, b_turn: 1 }, 'turn'],
  ] as const)('refuses %s', (_name, bad, mentions) => {
    const r = gameRecordFrom(bad);
    expect(r.ok).toBe(false);
    expect(!r.ok && r.message).toContain(mentions);
  });
});

describe('emptyGameForm', () => {
  it('starts with no turn chosen', () => {
    const f = emptyGameForm('2026-10-14', 'UTC');
    expect([f.a_turn, f.b_turn]).toEqual([null, null]);
  });
});

describe('chooseTurn', () => {
  it.each([
    ['the first chosen fills in the other', [null, null], 'a', 1, [1, 2]],
    ['the second side chosen first', [null, null], 'b', 1, [2, 1]],
    ["a turn the other holds swaps them", [1, 2], 'a', 2, [2, 1]],
    ['the same turn again changes nothing', [2, 1], 'a', 2, [2, 1]],
    ["a turn the other holds, one not chosen yet", [null, 1], 'a', 1, [1, 2]],
  ] as const)('%s', (_name, [a, b], who, turn, want) => {
    const f = chooseTurn({ ...form, a_turn: a, b_turn: b }, who, turn);
    expect([f.a_turn, f.b_turn]).toEqual(want);
  });
});
