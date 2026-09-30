import { describe, expect, it } from 'vitest';
import {
  BUILT,
  cultureScore,
  era,
  INTERNATIONAL,
  WORLDS,
  leaguePoints,
  medalCells,
  opponentsLine,
  seriesEra,
  SERIES_CONTENT,
  SERIES_NAV,
  seriesHref,
  seriesRecord,
} from './tournament-series';

describe('the series nav', () => {
  it('lists upstream’s tabs in its order', () => {
    expect(SERIES_NAV.map((s) => s.slug)).toEqual([
      'overview',
      'international',
      'intermezzo',
      'royal_league',
      'worlds',
      'grand_slams',
      'ladders',
    ]);
  });

  it('links a series to its page and the overview to the hub', () => {
    expect(seriesHref('international')).toBe('/ratings/hall_of_fame/international/');
    expect(seriesHref('overview')).toBe('/ratings/hall_of_fame/');
  });
});

describe('the series figures', () => {
  it('names the era by its latest season', () => {
    expect(era('2016', 34)).toBe('2016 – Present (Current: Season 34)');
  });

  it('writes points and scores as upstream does', () => {
    expect(leaguePoints(36)).toBe('36.0 pts');
    expect(leaguePoints(22.5)).toBe('22.5 pts');
    expect(cultureScore(388)).toBe('388 pts');
    expect(cultureScore(301.5)).toBe('302 pts');
  });

  it('lists opponents highest first, whole scores without a decimal', () => {
    expect(
      opponentsLine([
        { name: 'funestus', score: 334 },
        { name: 'wolvs', score: 307.5 },
      ]),
    ).toBe('vs funestus (334 pts), wolvs (307.5 pts)');
    expect(opponentsLine([])).toBe('');
  });

  it('shows a league season as its places, and a Royal League season as wins, draws and losses', () => {
    expect(seriesRecord('international', [6, 1, 0, 0])).toBe('6/1/0/0');
    expect(seriesRecord('intermezzo', [6, 0, 0])).toBe('6/0/0');
    expect(seriesRecord('royal_league', [6, 0, 1])).toBe('6/1');
    expect(seriesRecord('royal_league', [5, 1, 1])).toBe('5/1/1');
  });

  it('has a page for each series built so far', () => {
    const series = ['international', 'intermezzo', 'royal_league', 'worlds', 'grand_slams', 'ladders'];
    expect(SERIES_CONTENT.map((c) => c.slug)).toEqual(series);
    expect([...BUILT].sort()).toEqual([...series].sort());
  });

  it('states a static era where upstream does, else the latest season', () => {
    expect(seriesEra(INTERNATIONAL, 34)).toBe('2016 – Present (Current: Season 34)');
    expect(seriesEra(WORLDS, 5)).toBe('2023 – Present (Next: 2027)');
  });

  it('leaves a board’s medal cells empty where the board has no medals, as upstream', () => {
    expect(medalCells({ gold: 2, silver: 1, bronze: 0 })).toEqual(['2', '1', '0', '3']);
    expect(medalCells({ gold: null, silver: null, bronze: null })).toEqual(['', '', '', '']);
  });
});
