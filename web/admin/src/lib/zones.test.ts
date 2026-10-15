import { describe, expect, it } from 'vitest';
import { clockIn, zoneChoices, zoneFromText, type Region } from './zones';

const regions: Region[] = [
  { name: 'Unknown', zone: '', utc_offset: '' },
  { name: 'US West', zone: 'America/Los_Angeles', utc_offset: '-07:00' },
  { name: 'US East', zone: 'America/New_York', utc_offset: '-04:00' },
  { name: 'East Asia', zone: 'Asia/Shanghai', utc_offset: '+08:00' },
];
const known = ['America/Los_Angeles', 'America/New_York', 'Asia/Shanghai', 'Australia/Sydney', 'Europe/Berlin', 'Europe/Busingen'];

describe('zoneChoices', () => {
  it.each([
    ['nothing typed: Unknown and every region, in order', '', 3, ['Unknown', 'US West', 'US East', 'East Asia']],
    ['regions first, then zones, each the name, then names starting with it, then the rest', 'us', 9, ['US West', 'US East', 'Australia/Sydney', 'Europe/Busingen']],
    ['a region holding the text before a zone starting with it', 'asia', 9, ['East Asia', 'Asia/Shanghai']],
    ['an underscore read as a space, in any case', 'NEW YORK', 9, ['America/New_York']],
    ['the zone itself first', 'europe/berlin', 9, ['Europe/Berlin']],
    ['UTC though the browser does not list it', 'utc', 9, ['UTC']],
    ['no more than the limit', 'e', 2, ['East Asia', 'US West']],
    ['each word typed starting a word of the name, in order', 'u e', 9, ['US East']],
    ["a zone's area and city by their first letters", 'a s', 9, ['Asia/Shanghai', 'Australia/Sydney']],
    ['words typed in the order of the name', 'west us', 9, []],
    ['nothing that holds the text', 'mars', 9, []],
  ])('%s', (_, typed, limit, want) => {
    expect(zoneChoices(regions, known, typed, limit).map((c) => c.value)).toEqual(want);
  });

  it("says whose time a region keeps, and that Unknown pairs as UTC", () => {
    expect(zoneChoices(regions, known, '', 9).map((c) => c.detail)).toEqual(['not known: paired as UTC', 'Los Angeles time, UTC-7', 'New York time, UTC-4', 'Shanghai time, UTC+8']);
  });
});

describe('zoneFromText', () => {
  it.each([
    ['a region in any case', 'us east', 'US East'],
    ['a zone with a space for its underscore', 'america/new york', 'America/New_York'],
    ['Unknown', ' unknown ', 'Unknown'],
    ['UTC', 'utc', 'UTC'],
    ['part of a name', 'US', null],
    ['the start of each word of a name', 'us e', null],
    ['nothing', '', null],
  ])('%s', (_, text, want) => {
    expect(zoneFromText(regions, known, text)).toBe(want);
  });
});

describe('clockIn', () => {
  it.each([
    ['a region, on its city\'s clocks', 'US East', { day: '2026-10-14', clock: '15:30' }],
    ['an IANA zone, onto the next day', 'Australia/Sydney', { day: '2026-10-15', clock: '06:30' }],
    ['Unknown, on UTC', 'Unknown', { day: '2026-10-14', clock: '19:30' }],
  ])('reads an instant on %s', (_, zone, want) => {
    expect(clockIn(regions, zone, '2026-10-14T19:30:00Z')).toEqual(want);
  });
});
