import { describe, expect, it } from 'vitest';
import { formatLabel, offsetLabel, zoneLabel, zoneOptions } from './blitz';

describe('offsetLabel', () => {
  it.each([
    ['+02:00', 'UTC+2'],
    ['-04:00', 'UTC-4'],
    ['+05:30', 'UTC+5:30'],
    ['+00:00', 'UTC'],
    ['', ''],
  ])('shows %s as %s', (offset, want) => {
    expect(offsetLabel(offset)).toBe(want);
  });
});

describe('zoneLabel', () => {
  it('puts the offset before the zone', () => {
    expect(zoneLabel('Europe/Berlin', '+02:00')).toBe('UTC+2 · Europe/Berlin');
  });
  it('shows the zone alone when the offset is unknown', () => {
    expect(zoneLabel('Mars/Olympus', '')).toBe('Mars/Olympus');
  });
});

describe('formatLabel', () => {
  it.each([
    [{ swiss_rounds: 8, knockout_size: 8 }, '8 Swiss rounds, then a top-8 knockout'],
    [{ swiss_rounds: 1, knockout_size: 4 }, '1 Swiss round, then a top-4 knockout'],
    [{ swiss_rounds: 6, knockout_size: 0 }, '6 Swiss rounds, no knockout'],
  ])('describes %o', (format, want) => {
    expect(formatLabel(format)).toBe(want);
  });
});

describe('zoneOptions', () => {
  it('puts UTC first, the rest in order, and the current zone in when the list lacks it', () => {
    expect(zoneOptions(['Europe/Berlin', 'UTC', 'America/New_York'], 'Asia/Tokyo')).toEqual([
      'UTC',
      'America/New_York',
      'Asia/Tokyo',
      'Europe/Berlin',
    ]);
  });
  it('does not repeat a current zone the list has, nor an empty one', () => {
    expect(zoneOptions(['Europe/Berlin'], 'Europe/Berlin')).toEqual(['UTC', 'Europe/Berlin']);
    expect(zoneOptions(['Europe/Berlin'], '')).toEqual(['UTC', 'Europe/Berlin']);
  });
});
