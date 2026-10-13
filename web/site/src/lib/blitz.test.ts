import { describe, expect, it } from 'vitest';
import { editionHref, editionParams, ratingText, splitRounds } from './blitz';

describe('splitRounds', () => {
  it('puts the Swiss rounds newest first and the knockout rounds in bracket order, first to final', () => {
    const rounds = [
      { number: 1, kind: 'swiss' },
      { number: 4, kind: 'knockout' },
      { number: 3, kind: 'knockout' },
      { number: 2, kind: 'swiss' },
    ] as const;
    const { swiss, knockout } = splitRounds(rounds);
    expect(swiss.map((r) => r.number)).toEqual([2, 1]);
    expect(knockout.map((r) => r.number)).toEqual([3, 4]);
  });
});

describe('editionParams', () => {
  it.each([
    ['both names', '?t=us_open&e=2026', { slug: 'us_open', edition: '2026' }],
    ['an edition with spaces', '?t=us_open&e=Season+1', { slug: 'us_open', edition: 'Season 1' }],
    ['no edition', '?t=us_open', null],
    ['no tournament', '?e=2026', null],
    ['a slug that is not one', '?t=Bad!&e=2026', null],
    ['nothing', '', null],
  ])('reads %s', (_name, search, want) => {
    expect(editionParams(new URLSearchParams(search))).toEqual(want);
  });
});

describe('editionHref', () => {
  it('escapes the edition', () => {
    expect(editionHref('us_open', 'Season 1')).toBe('/blitz/?t=us_open&e=Season%201');
  });
});

describe('ratingText', () => {
  it.each([
    [1712.4, '1712'],
    [1699.5, '1700'],
    [null, '–'],
  ])('shows %s as %s', (rating, want) => {
    expect(ratingText(rating)).toBe(want);
  });
});
