import { describe, expect, it } from 'vitest';
import { formatName, modelLabel, rank, rating, cRating, rd, winRate, isTopThree, titleName, flagSrc } from './leaderboard-display';

describe('formatName', () => {
  it.each([
    [0, 'All Formats'],
    [2, '2-Player (Duel)'],
    [3, '3-Player'],
    [4, '4-Player'],
  ] as const)('names format %i as upstream does', (format, name) => {
    expect(formatName(format)).toBe(name);
  });
});

describe('modelLabel', () => {
  it.each([
    ['glicko2_daneo', 'GlickoD'],
    ['glicko2_daneo_softer', 'GlickoD · Season Reset'],
    ['glicko2_std', 'Glicko-2 Standard'],
    ['glicko2_std_retro', 'Glicko-2 Standard · 15G Retro'],
    ['glicko2_mp_softer_retro', 'Glicko-2 MP-Weighted · Season Reset · 15G Retro'],
    ['glicko2_adapt', 'Glicko-2 Adaptive-T'],
    ['whr_softer', 'Whole-History Rating · Season Reset'],
  ] as const)('labels %s with upstream engine and mode names', (model, label) => {
    expect(modelLabel(model)).toBe(label);
  });
});

describe('numbers', () => {
  it('prefixes a rank with #', () => {
    expect(rank(7)).toBe('#7');
  });

  it('shows a dash for a player with no rank', () => {
    expect(rank(null)).toBe('–');
  });

  it('marks only ranks 1 to 3 as top three', () => {
    expect([1, 2, 3, 4].map(isTopThree)).toEqual([true, true, true, false]);
    expect(isTopThree(null)).toBe(false);
  });

  it('shows the conservative rating to two places and the rating to one', () => {
    expect(cRating(1834.456)).toBe('1834.46');
    expect(rating(2012.04)).toBe('2012.0');
  });

  it('shows the deviation with a plus-minus sign', () => {
    expect(rd(59.26)).toBe('±59.3');
  });

  it('shows the win rate as a percentage to one place', () => {
    expect(winRate(61.25)).toBe('61.3%');
    expect(winRate(0)).toBe('0.0%');
  });
});

describe('titleName', () => {
  it.each([
    ['WC', 'World Champion'],
    ['GM', 'Grandmaster'],
    ['M', 'Master'],
    ['P', 'Platinum'],
    ['G', 'Gold'],
    ['S', 'Silver'],
    ['B', 'Bronze'],
    ['W', 'Wood'],
  ] as const)('names %s as upstream does', (code, name) => {
    expect(titleName(code)).toBe(name);
  });
});

describe('flagSrc', () => {
  it('points at the lower-case SVG for a country code', () => {
    expect(flagSrc('PL')).toBe('/flags/pl.svg');
  });
});
