import { describe, expect, it } from 'vitest';
import { formatName, modelLabel, rank, rating, cRating, rd, winRate, isTopThree, titleName, titleBadge, badgeHover, flagSrc, engineName, resetModeName, engineNote, delta } from './leaderboard-display';

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

describe('engineName and resetModeName', () => {
  it.each([
    ['glicko2_daneo', 'GlickoD', 'Continuous (Career)'],
    ['glicko2_daneo_softer', 'GlickoD', 'Season Reset'],
    ['glicko2_std_softer_retro', 'Glicko-2 Standard', 'Season Reset'],
    ['whr', 'Whole-History Rating', 'Continuous (Career)'],
  ] as const)('splits %s into upstream engine and reset mode names', (model, engine, mode) => {
    expect(engineName(model)).toBe(engine);
    expect(resetModeName(model)).toBe(mode);
  });
});

describe('engineNote', () => {
  it.each([
    ['glicko2_daneo', ''],
    ['glicko2_std_softer', 'Standard 1v1 naive baseline'],
    ['glicko2_mp', 'Multiplayer fractional weighting — variance-calibrated'],
    ['glicko2_adapt_retro', 'Adaptive temperature scaling'],
    ['whr', 'Whole-History Rating retrospective MAP'],
  ] as const)('gives %s upstream note %j', (model, note) => {
    expect(engineNote(model)).toBe(note);
  });
});

describe('delta', () => {
  it.each([
    [12.34, '↑+12.3', 'delta-pos'],
    [-3.21, '↓-3.2', 'delta-neg'],
    [0.05, '—0.0', 'delta-zero'],
    [-0.05, '—0.0', 'delta-zero'],
  ] as const)('shows %d as upstream: %s', (value, text, kind) => {
    expect(delta(value)).toEqual({ text, kind });
  });
});

describe('titleBadge', () => {
  it("shows a title's code, with the run's reason on hover", () => {
    expect(titleBadge({ code: 'GM', reason: 'Played in Royal League' })).toEqual({
      code: 'GM',
      text: 'GM',
      hover: 'Played in Royal League',
    });
  });

  it("falls back to the title's full name on hover when the run gave no reason", () => {
    expect(titleBadge({ code: 'WC', reason: '' })?.hover).toBe('World Champion');
  });

  it('shows a Super GM as GM, in its own colour, as upstream', () => {
    expect(titleBadge({ code: 'SGM', reason: 'r' })).toMatchObject({ code: 'SGM', text: 'GM' });
  });

  it('gives nothing for a player without a title', () => {
    expect(titleBadge(null)).toBeNull();
  });
});

describe('badgeHover', () => {
  it("shows an admin badge's description on hover, or its label when it has none", () => {
    expect(badgeHover({ label: 'Organiser', description: 'Runs a league' })).toBe('Runs a league');
    expect(badgeHover({ label: 'TD', description: '' })).toBe('TD');
  });
});
