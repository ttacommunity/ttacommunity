import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { clockIn, clockOrUtc, formatDate, formatDay, formatInstant, formatTime, knownZones, shownZone, todayIn } from './time';

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

describe.each(['America/New_York', 'UTC', 'Australia/Sydney'])('formatDay with the viewer in %s', (zone) => {
  let saved: string | undefined;
  beforeEach(() => {
    saved = process.env.TZ;
    process.env.TZ = zone;
  });
  afterEach(() => {
    process.env.TZ = saved;
  });

  it.each([
    ['a winter day', '2025-02-24', '24 Feb 2025'],
    ['New Year', '2026-01-01', '1 Jan 2026'],
    ['New Year\'s Eve', '2025-12-31', '31 Dec 2025'],
  ])('shows %s as that day', (_name, day, want) => {
    expect(formatDay(day)).toBe(want);
  });
});

describe('todayIn', () => {
  it.each([
    ['Sydney, already tomorrow', '2026-10-27T14:30:00Z', 'Australia/Sydney', '2026-10-28'],
    ['Sydney, the evening before its clocks go forward', '2026-10-03T13:30:00Z', 'Australia/Sydney', '2026-10-03'],
    ['Los Angeles, still yesterday', '2026-10-28T03:00:00Z', 'America/Los_Angeles', '2026-10-27'],
    ['London, just after midnight in summer time', '2026-07-01T23:30:00Z', 'Europe/London', '2026-07-02'],
    ['a region, on the browser’s clocks', '2026-10-27T14:30:00Z', 'Europe', '2026-10-27'],
    ['Unknown, on the browser’s clocks', '2026-10-27T14:30:00Z', 'Unknown', '2026-10-27'],
  ])('gives the day in %s', (_name, now, zone, want) => {
    expect(todayIn(zone, new Date(now), 'Europe/Berlin')).toBe(want);
  });
});

describe('formatDate', () => {
  it.each([
    ['in the zone given', '2025-02-24T23:30:00Z', 'UTC', '24 Feb 2025'],
    ['the next day where it already is', '2025-02-24T23:30:00Z', 'Europe/Berlin', '25 Feb 2025'],
  ])('writes the day %s', (_name, instant, zone, want) => {
    expect(formatDate(instant, zone)).toBe(want);
  });
});

describe('clockIn', () => {
  it.each([
    ['an IANA zone, onto the next day', 'Australia/Sydney', { day: '2026-10-15', clock: '06:30' }],
    ['UTC', 'UTC', { day: '2026-10-14', clock: '19:30' }],
    ['a region, which the browser cannot read', 'US East', null],
    ['Unknown', 'Unknown', null],
  ])('reads an instant on %s', (_name, zone, want) => {
    expect(clockIn(zone, '2026-10-14T19:30:00Z')).toEqual(want);
  });
});

describe('clockOrUtc', () => {
  it.each([
    ['an IANA zone, on its clocks', 'Australia/Sydney', { day: '2026-10-15', clock: '06:30' }],
    ['a zone the browser cannot read, on UTC', 'Unknown', { day: '2026-10-14', clock: '19:30' }],
  ])('reads an instant in %s', (_name, zone, want) => {
    expect(clockOrUtc(zone, '2026-10-14T19:30:00Z')).toEqual(want);
  });
});

describe('shownZone', () => {
  it.each([
    ['a saved IANA zone', 'Pacific/Auckland', 'Pacific/Auckland'],
    ['a region, which is not a zone', 'US East', 'Europe/Berlin'],
    ['Unknown', 'Unknown', 'Europe/Berlin'],
  ])('shows times in %s', (_name, saved, want) => {
    expect(shownZone(saved, 'Europe/Berlin')).toBe(want);
  });
});

describe('knownZones', () => {
  it('lists the zones the runtime knows', () => {
    expect(knownZones()).toContain('Europe/Berlin');
  });
});
