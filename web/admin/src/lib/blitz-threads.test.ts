import { describe, expect, it } from 'vitest';
import { deadlineForm, deadlineLabel, deadlineLocked, organiserChoices, threadTextSavable, threadsBlocker, threadsFileName } from './blitz-threads';
import type { Candidate } from './person-form';
import type { Region } from './zones';

const regions: Region[] = [
  { name: 'Unknown', zone: '', utc_offset: '' },
  { name: 'US East', zone: 'America/New_York', utc_offset: '-04:00' },
];
const deadline = { at: '2026-10-19T12:00:00Z', zone: 'Europe/London' };
const game = { b: 8 };
const bye = { b: null };

describe('threadsBlocker', () => {
  it.each([
    ['ready: a deadline, an organiser and a game', { deadline, matches: [game, bye] }, 1, false, ''],
    ['changes not saved yet', { deadline, matches: [game] }, 1, true, 'Save or discard the changes first.'],
    ['no deadline', { deadline: null, matches: [game] }, 1, false, 'Set the deadline first.'],
    ['no organiser', { deadline, matches: [game] }, 0, false, 'Add the organisers first, under Organisers.'],
    ['only a bye', { deadline, matches: [bye] }, 1, false, 'No games to make threads for.'],
  ])('%s', (_, round, organisers, unsaved, want) => {
    expect(threadsBlocker(round, organisers, unsaved)).toBe(want);
  });
});

describe('threadsFileName', () => {
  it.each([
    ['a plain label', 'us_open', '2026', 3, 'us_open-2026-round-3-threads.txt'],
    ['spaces and other characters as one dash', 'us_open', 'Spring 2026 (B)', 9, 'us_open-Spring-2026-B--round-9-threads.txt'],
  ])('%s', (_, slug, edition, round, want) => {
    expect(threadsFileName(slug, edition, round)).toBe(want);
  });
});

describe('deadlineForm', () => {
  it.each([
    ['a deadline on the clocks of the zone it was set in', deadline, 'Asia/Tokyo', { day: '2026-10-19', time: '13:00', zone: 'Europe/London' }],
    ['a region keeps its city’s clocks', { at: '2026-10-19T16:00:00Z', zone: 'US East' }, 'UTC', { day: '2026-10-19', time: '12:00', zone: 'US East' }],
    ['none yet: empty, in the zone given', null, 'Asia/Tokyo', { day: '', time: '', zone: 'Asia/Tokyo' }],
  ])('%s', (_, d, zone, want) => {
    expect(deadlineForm(regions, d, zone)).toEqual(want);
  });
});

describe('deadlineLabel', () => {
  it.each([
    ['the day and time where it was set, and that zone', deadline, 'deadline 2026-10-19 13:00 Europe/London'],
    ['none yet', null, 'no deadline'],
  ])('%s', (_, d, want) => {
    expect(deadlineLabel(regions, d)).toBe(want);
  });
});

describe('organiserChoices', () => {
  const person = (member: number | null, name: string, discord_name: string): Candidate => ({ member, name, player: '', discord_name, email: '', country: null, zone: null, banned: false });
  it('offers members by name with their Discord username, a player who is no member as not to pick, and leaves out who organises it already', () => {
    const found = [person(21, 'Carol', 'carol_tta'), person(22, 'Dave', ''), person(null, 'Ed', ''), person(23, 'Fay', 'fay')];
    expect(organiserChoices(found, [{ member: 23 }])).toEqual([
      { key: '21', label: 'Carol', detail: '@carol_tta', disabled: false },
      { key: '22', label: 'Dave', detail: 'no Discord username yet', disabled: false },
      { key: 'player-Ed', label: 'Ed', detail: 'not a member: add them under Members first', disabled: true },
    ]);
  });
});

describe('deadlineLocked', () => {
  const played = { b: 8, result: 'a' };
  const toPlay = { b: 8, result: null };
  const byeOnly = { b: null, result: null };
  it.each([
    ['every game has a result, a bye aside', [played, byeOnly], true],
    ['a game still to play', [played, toPlay], false],
    ['no game at all', [byeOnly], false],
    ['no matches yet', [], false],
  ])('%s', (_, matches, want) => {
    expect(deadlineLocked({ matches })).toBe(want);
  });
});

describe('threadTextSavable', () => {
  const saved = { message: 'Hi {players}.', no_discord: '{name}.' };
  it.each([
    ['unchanged', saved, false],
    ['only spaces added around it', { message: ' Hi {players}.\n', no_discord: '{name}. ' }, false],
    ['a new message', { ...saved, message: 'Hello {players}.' }, true],
    ['a new no-Discord sentence', { ...saved, no_discord: 'Friend {name}.' }, true],
    ['an empty message', { ...saved, message: '  ' }, false],
    ['an empty sentence', { ...saved, no_discord: '' }, false],
  ])('%s', (_, form, want) => {
    expect(threadTextSavable(form, saved)).toBe(want);
  });
});
