import { describe, expect, it } from 'vitest';
import { settingsHref, slugParam } from './tournament-page';

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
