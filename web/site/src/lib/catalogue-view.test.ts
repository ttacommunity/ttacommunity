import { describe, expect, it } from 'vitest';
import { catalogueHref, placeLabel, spanYears, stageName, tournamentParam } from './catalogue-view';

describe('tournamentParam', () => {
  it('takes a slug from the address', () => {
    expect(tournamentParam(new URLSearchParams('t=slow_burn'))).toBe('slow_burn');
  });
  it('gives null for no slug or one the API would refuse', () => {
    for (const q of ['', 't=', 't=Wimbledon', 't=9cup', 't=a%2Fb', `t=${'a'.repeat(65)}`]) {
      expect(tournamentParam(new URLSearchParams(q)), q).toBeNull();
    }
  });
});

describe('catalogueHref', () => {
  it('links the list, or one tournament', () => {
    expect(catalogueHref()).toBe('/ratings/tournaments/');
    expect(catalogueHref('french_open')).toBe('/ratings/tournaments/?t=french_open');
  });
});

describe('spanYears', () => {
  it('gives one year, or the first and last', () => {
    expect(spanYears('2024-07-01', '2024-12-20')).toBe('2024');
    expect(spanYears('2019-03-10', '2026-05-31')).toBe('2019 – 2026');
  });
});

describe('stageName', () => {
  it('names a season\'s only stage', () => {
    expect(stageName('')).toBe('Main stage');
    expect(stageName('Stage 4')).toBe('Stage 4');
  });
});

describe('placeLabel', () => {
  it('names the podium places as the Hall of Fame does', () => {
    expect([1, 2, 3].map(placeLabel)).toEqual(['🥇 Champion', '🥈 Runner-Up', '🥉 3rd Place']);
  });
});
