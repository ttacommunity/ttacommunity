import { describe, expect, it } from 'vitest';
import { panelsFor, tabFrom, tabsFor } from './admin-tabs';

describe('admin tabs', () => {
  it.each([
    ['admin', ['tournaments', 'community', 'data', 'titles', 'admins']],
    ['organiser', ['tournaments', 'community', 'data']],
  ] as const)('shows %s its tabs', (role, want) => {
    expect(tabsFor(role).map((t) => t.key)).toEqual(want);
  });

  it.each([
    ['#titles', 'admin', 'titles'],
    ['#community', 'organiser', 'community'],
    ['', 'admin', 'tournaments'],
    ['#nonsense', 'admin', 'tournaments'],
    ['#runs', 'admin', 'data'],
    ['#admins', 'organiser', 'tournaments'],
    ['#data', 'organiser', 'data'],
  ] as const)('opens %s for an %s on %s', (hash, role, want) => {
    expect(tabFrom(hash, role)).toBe(want);
  });

  it.each([
    ['community', 'admin', ['community']],
    ['community', 'organiser', ['community']],
    ['data', 'admin', ['data', 'runs', 'standings']],
    ['data', 'organiser', ['data', 'standings']],
    ['titles', 'admin', ['titles', 'badges']],
    ['tournaments', 'organiser', ['tournaments']],
    ['admins', 'admin', ['admins']],
  ] as const)('shows the %s tab to an %s as its panels', (tab, role, want) => {
    expect(panelsFor(tab, role)).toEqual(want);
  });
});
