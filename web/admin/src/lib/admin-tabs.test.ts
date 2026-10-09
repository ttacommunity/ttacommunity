import { describe, expect, it } from 'vitest';
import { tabFrom, tabsFor } from './admin-tabs';

describe('admin tabs', () => {
  it.each([
    ['admin', ['tournaments', 'data', 'titles', 'admins']],
    ['organiser', ['tournaments', 'data']],
  ] as const)('shows %s its tabs', (role, want) => {
    expect(tabsFor(role).map((t) => t.key)).toEqual(want);
  });

  it.each([
    ['#titles', 'admin', 'titles'],
    ['', 'admin', 'tournaments'],
    ['#nonsense', 'admin', 'tournaments'],
    ['#runs', 'admin', 'data'],
    ['#admins', 'organiser', 'tournaments'],
    ['#data', 'organiser', 'data'],
  ] as const)('opens %s for an %s on %s', (hash, role, want) => {
    expect(tabFrom(hash, role)).toBe(want);
  });
});
