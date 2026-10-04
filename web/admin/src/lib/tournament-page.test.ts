import { describe, expect, it } from 'vitest';
import { settingsHref, slugParam, toggledCounts } from './tournament-page';

describe('settingsHref and slugParam', () => {
  it('link to a tournament settings page and read it back', () => {
    expect(settingsHref('royal_league')).toBe('/tournament/?t=royal_league');
    expect(slugParam(new URLSearchParams('t=royal_league'))).toBe('royal_league');
  });

  it('refuse an address that names no tournament the API would accept', () => {
    for (const q of ['', 't=', 't=Worlds', 't=../me', 't=' + 'a'.repeat(65)]) {
      expect(slugParam(new URLSearchParams(q))).toBeNull();
    }
  });
});

describe('toggledCounts', () => {
  it('adds or removes a player count, kept in order', () => {
    expect(toggledCounts([2, 4], 3, true)).toEqual([2, 3, 4]);
    expect(toggledCounts([2, 3, 4], 2, false)).toEqual([3, 4]);
  });

  it('refuses to leave no player count rated (turn the tournament off instead)', () => {
    expect(toggledCounts([3], 3, false)).toBeNull();
  });
});
