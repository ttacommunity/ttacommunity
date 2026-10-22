import { describe, expect, it } from 'vitest';
import { formatInstant, formatTime } from './time';

describe('formatInstant', () => {
  it.each([
    ['London in summer time', '2026-07-01T12:00:00Z', 'Europe/London', '1 Jul 2026, 13:00 (Europe/London)'],
    ['London after the clocks go back', '2026-10-25T12:00:00Z', 'Europe/London', '25 Oct 2026, 12:00 (Europe/London)'],
    ['Sydney before the clocks go forward', '2026-10-03T12:00:00Z', 'Australia/Sydney', '3 Oct 2026, 22:00 (Australia/Sydney)'],
    ['Sydney after the clocks go forward', '2026-10-05T12:00:00Z', 'Australia/Sydney', '5 Oct 2026, 23:00 (Australia/Sydney)'],
    ['a date line crossing', '2026-01-01T11:30:00Z', 'Pacific/Kiritimati', '2 Jan 2026, 01:30 (Pacific/Kiritimati)'],
  ])('shows %s', (_name, instant, zone, want) => {
    expect(formatInstant(instant, zone)).toBe(want);
  });

  it('refuses an unknown zone', () => {
    expect(() => formatInstant('2026-07-01T12:00:00Z', 'Mars/Olympus')).toThrow(RangeError);
  });

  it.each([
    ['a local time with no zone', '2026-07-01T12:00:00'],
    ['an offset instead of UTC', '2026-07-01T12:00:00+02:00'],
    ['garbage', 'tomorrow'],
  ])('refuses %s', (_name, instant) => {
    expect(() => formatInstant(instant, 'Europe/London')).toThrow(RangeError);
  });
});

describe('formatTime', () => {
  it.each([
    ['Sydney after the clocks go forward', '2026-10-05T12:00:00Z', 'Australia/Sydney', '5 Oct 2026, 23:00'],
    ['Berlin, with the fraction Go sends', '2026-10-20T12:02:35.005172Z', 'Europe/Berlin', '20 Oct 2026, 14:02'],
  ])('shows %s without its zone', (_name, instant, zone, want) => {
    expect(formatTime(instant, zone)).toBe(want);
  });

  it('refuses an instant that is not UTC', () => {
    expect(() => formatTime('2026-10-20T23:02:35+11:00', 'Australia/Sydney')).toThrow(RangeError);
  });
});
