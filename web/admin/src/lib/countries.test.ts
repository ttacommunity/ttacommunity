import { describe, expect, it } from 'vitest';
import { countryChoices, countryFromText, countryNames } from './countries';

const countries = [
  { code: 'DE', name: 'Germany' },
  { code: 'GE', name: 'Georgia' },
  { code: 'NE', name: 'Niger' },
  { code: 'NG', name: 'Nigeria' },
  { code: 'GB', name: 'United Kingdom' },
  { code: 'US', name: 'United States' },
];

describe('countryChoices', () => {
  it.each([
    ['a code typed in full first, then names starting with it, then holding it', 'ge', 9, ['GE', 'DE', 'NE', 'NG']],
    ['a code alone', 'de', 9, ['DE']],
    ['each word typed starting a word of the name', 'u s', 9, ['US']],
    ['names starting alike, by name', 'nig', 9, ['NE', 'NG']],
    ['no more than the limit, by name', 'e', 2, ['GE', 'DE']],
    ['nothing for nothing typed', ' ', 9, []],
  ])('%s', (_, typed, limit, want) => {
    expect(countryChoices(countries, typed, limit).map((c) => c.code)).toEqual(want);
  });
});

describe('countryFromText', () => {
  it.each([
    ['a name in full, in any case', 'germany', 'DE'],
    ['a code', ' gb ', 'GB'],
    ['none for no text', ' ', ''],
    ['nothing for part of a name', 'germ', null],
  ])('%s', (_, text, want) => {
    expect(countryFromText(countries, text)).toBe(want);
  });
});

describe('countryNames', () => {
  it('names each code in English, sorted by name', () => {
    expect(countryNames(['NZ', 'DE'])).toEqual([
      { code: 'DE', name: 'Germany' },
      { code: 'NZ', name: 'New Zealand' },
    ]);
  });
});
