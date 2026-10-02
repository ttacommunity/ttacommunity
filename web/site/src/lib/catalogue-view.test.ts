import { describe, expect, it } from 'vitest';
import { catalogueHref, cgeYears, gameWhere, outcomeNote, placeLabel, seasonName, shownName, spanYears, stageName, tournamentParam } from './catalogue-view';

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

describe('seasonName', () => {
  it('calls a numbered or dated season a season', () => {
    expect(seasonName('24')).toBe('Season 24');
    expect(seasonName('2022 (Age A)')).toBe('Season 2022 (Age A)');
    expect(seasonName('1 (2024)')).toBe('Season 1 (2024)');
  });
  it('writes a named season as it is', () => {
    expect(seasonName('First Edition (2023)')).toBe('First Edition (2023)');
    expect(seasonName('Grand Prix 2023')).toBe('Grand Prix 2023');
  });
});

describe('placeLabel', () => {
  it('names the podium places as the Hall of Fame does', () => {
    expect([1, 2, 3].map(placeLabel)).toEqual(['🥇 Champion', '🥈 Runner-Up', '🥉 3rd Place']);
  });
});

describe('cgeYears', () => {
  it('gives the years a CGE tournament ran, or when it began if it has not finished', () => {
    expect(cgeYears('2024-06-01', '2025-02-24')).toBe('2024 – 2025');
    expect(cgeYears(null, '2023-05-23')).toBe('2023');
    expect(cgeYears('2026-09-29', null)).toBe('2026 –');
    expect(cgeYears(null, null)).toBe('');
  });
});

describe('shownName', () => {
  it('marks a tournament shown though not rated', () => {
    expect(shownName('Nations Cup', false)).toBe('Nations Cup (Not Rated)');
    expect(shownName('Wimbledon 2024', true)).toBe('Wimbledon 2024');
  });
});

describe('gameWhere', () => {
  it('names the stage, group and game a game sits in, leaving out what it lacks', () => {
    expect(gameWhere({ stage: 'Stage 2', group: 'Lower 17', game: 5 })).toBe('Stage 2 · Lower 17 · game 5');
    expect(gameWhere({ stage: '', group: 'Bronze 3', game: 7 })).toBe('Bronze 3 · game 7');
    expect(gameWhere({ stage: 'Stage 1', group: '', game: null })).toBe('Stage 1');
    expect(gameWhere({ stage: '', group: '', game: null })).toBe('');
  });
});

describe('outcomeNote', () => {
  it('marks a seat that did not finish', () => {
    expect(outcomeNote('resigned')).toBe('resigned');
    expect(outcomeNote('timed_out')).toBe('timed out');
    expect(outcomeNote('finished')).toBe('');
    expect(outcomeNote(null)).toBe('');
  });
});
