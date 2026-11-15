import { describe, expect, it } from 'vitest';
import { countryName, countryNames } from './countries';

describe('countryNames', () => {
  const names = { of: (code: string) => ({ DE: 'Germany', NZ: 'New Zealand' })[code] } as Intl.DisplayNames;
  it.each([
    ['names, sorted by name', ['NZ', 'DE'], names, [{ code: 'DE', name: 'Germany' }, { code: 'NZ', name: 'New Zealand' }]],
    ['the code where the browser has no name', ['XK'], names, [{ code: 'XK', name: 'XK' }]],
    ['codes alone without names', ['NZ', 'DE'], null, [{ code: 'DE', name: 'DE' }, { code: 'NZ', name: 'NZ' }]],
  ])('%s', (_name, codes, given, want) => {
    expect(countryNames(codes, given)).toEqual(want);
  });

  it('names in English by default', () => {
    expect(countryNames(['NZ', 'DE'])).toEqual([
      { code: 'DE', name: 'Germany' },
      { code: 'NZ', name: 'New Zealand' },
    ]);
  });
});

describe('countryName', () => {
  it.each([
    ['a country in English', 'NZ', 'New Zealand'],
    ['the code where there is no name', 'ZZZ', 'ZZZ'],
  ])('%s', (_name, code, want) => {
    expect(countryName(code)).toBe(want);
  });
});
