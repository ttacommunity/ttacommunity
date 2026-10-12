import { describe, expect, it } from 'vitest';
import { editionHref, editionParams, playersView } from './blitz-page';

describe('editionParams', () => {
  it.each([
    ['both names', '?t=us_open&e=2026', { slug: 'us_open', edition: '2026' }],
    ['an edition with spaces', '?t=us_open&e=Season+1', { slug: 'us_open', edition: 'Season 1' }],
    ['no edition', '?t=us_open', null],
    ['no tournament', '?e=2026', null],
    ['a slug that is not one', '?t=Bad!&e=2026', null],
    ['an empty edition', '?t=us_open&e=', null],
  ])('reads %s', (_name, search, want) => {
    expect(editionParams(new URLSearchParams(search))).toEqual(want);
  });
});

describe('editionHref', () => {
  it('escapes the edition', () => {
    expect(editionHref('us_open', 'Season 1')).toBe('/blitz/?t=us_open&e=Season%201');
  });
});

describe('playersView', () => {
  it('keeps the active entrants, without Discord names or emails', () => {
    const entrants = [
      { id: 1, member: 11, name: 'Ann', player: 'Ann', discord_name: 'ann#1', email: 'ann@example.test', country: 'DE', zone: 'Europe/Berlin', utc_offset: '+02:00', rating: 1700, status: 'active' as const },
      { id: 2, member: 12, name: 'bob', player: '', discord_name: 'bobby', email: '', country: null, zone: 'UTC', utc_offset: '+00:00', rating: null, status: 'dropped' as const },
    ];
    expect(playersView(entrants)).toEqual([{ name: 'Ann', country: 'DE', zone: 'Europe/Berlin', utc_offset: '+02:00', rating: 1700 }]);
  });
});
