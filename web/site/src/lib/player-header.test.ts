import { describe, expect, it } from 'vitest';
import { headerPeak, headerTitle } from './player-header';

describe('headerTitle', () => {
  it.each([
    ['the reigning champion', { code: 'WC', count: 1, reason: '' }, { code: 'WC', text: 'Reigning World Champion', hover: 'Reigning World Champion' }],
    ['a Super GM, past champion or not', { code: 'SGM', count: 1, reason: '' }, { code: 'SGM', text: 'Super GM', hover: 'Super GM' }],
    ['a Grandmaster, with the reason as its hover', { code: 'GM', count: 1, reason: 'Won the 2025 Royal League' }, { code: 'GM', text: 'Grandmaster', hover: 'Won the 2025 Royal League' }],
  ] as const)('names %s by their current title', (_, title, want) => {
    expect(headerTitle(title)).toEqual(want);
  });

  it('shows no title for a player without one', () => {
    expect(headerTitle(null)).toBeNull();
  });
});

describe('headerPeak', () => {
  it.each([
    ['a past world title, with its year', { code: 'WC', year: 2024 }, { code: 'WC', text: 'World Champion (2024)' }],
    ['a career peak with no year', { code: 'GM', year: null }, { code: 'GM', text: 'Grandmaster (Career)' }],
  ] as const)('writes %s', (_, peak, want) => {
    expect(headerPeak(peak)).toEqual(want);
  });

  it('shows no peak for a player without one', () => {
    expect(headerPeak(null)).toBeNull();
  });
});
