import { describe, expect, it } from 'vitest';
import { deadlineForm, deadlineLabel, deadlineLocked, discordBlocker } from './blitz-deadlines';
import type { Region } from './zones';

const regions: Region[] = [
  { name: 'Unknown', zone: '', utc_offset: '' },
  { name: 'US East', zone: 'America/New_York', utc_offset: '-04:00' },
];
const deadline = { at: '2026-10-19T12:00:00Z', zone: 'Europe/London' };
const game = { b: 8 };
const bye = { b: null };

describe('discordBlocker', () => {
  it.each([
    ['ready: a game', { matches: [game, bye] }, false, ''],
    ['changes not saved yet', { matches: [game] }, true, 'Save or discard the changes first.'],
    ['only a bye', { matches: [bye] }, false, 'No games to write.'],
  ])('%s', (_, round, unsaved, want) => {
    expect(discordBlocker(round, unsaved)).toBe(want);
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

