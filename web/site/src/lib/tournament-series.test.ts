import { describe, expect, it } from 'vitest';
import { cultureScore, era, leaguePoints, opponentsLine, SERIES_NAV, seriesHref, seriesRecord } from './tournament-series';

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

  it('shows an International season as 1st/2nd/3rd/4th places', () => {
    expect(seriesRecord([6, 1, 0, 0])).toBe('6/1/0/0');
  });
});
