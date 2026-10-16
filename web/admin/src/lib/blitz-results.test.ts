import { describe, expect, it } from 'vitest';
import { gameRecordFrom, type GameForm } from './blitz-results';

const form: GameForm = { share_code: ' ABCD1234 ', a_culture: '250', a_outcome: 'finished', b_culture: '200', b_outcome: 'finished', finished_on: '2026-10-14', finished_time: '21:30', zone: 'Europe Central' };

describe('gameRecordFrom', () => {
  it('turns the form into the request, trimmed and numbered', () => {
    expect(gameRecordFrom(form)).toEqual({
      ok: true,
      body: { share_code: 'ABCD1234', a: { culture: 250, outcome: 'finished' }, b: { culture: 200, outcome: 'finished' }, finished_on: '2026-10-14', finished_time: '21:30', zone: 'Europe Central' },
    });
  });
  it('sends no share code for a game without one', () => {
    const r = gameRecordFrom({ ...form, share_code: ' ' });
    expect(r.ok && r.body.share_code).toBe('');
  });
  it('sends 0 for a player who resigned or timed out, whatever was typed', () => {
    const r = gameRecordFrom({ ...form, b_culture: '190', b_outcome: 'resigned' });
    expect(r.ok && r.body.b).toEqual({ culture: 0, outcome: 'resigned' });
  });
  it.each([
    ['a culture that is not a number', { ...form, a_culture: 'lots' }, 'culture'],
    ['a negative culture', { ...form, b_culture: '-1' }, 'culture'],
    ['an empty culture for a finished player', { ...form, a_culture: '' }, 'culture'],
    ['no day it finished', { ...form, finished_on: '' }, 'day'],
    ['no time it finished', { ...form, finished_time: '' }, 'time it finished'],
    ['no time zone', { ...form, zone: '' }, 'time zone'],
    ['nobody finished', { ...form, a_outcome: 'resigned', b_outcome: 'timed_out' }, 'finished'],
  ] as const)('refuses %s', (_name, bad, mentions) => {
    const r = gameRecordFrom(bad);
    expect(r.ok).toBe(false);
    expect(!r.ok && r.message).toContain(mentions);
  });
});
