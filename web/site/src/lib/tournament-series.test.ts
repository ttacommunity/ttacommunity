import { describe, expect, it } from 'vitest';
import type { Legend, SeriesCount } from '@ttac/shared/ratings';
import {
  BUILT,
  cultureScore,
  derivedMatches,
  legendMark,
  majorMedals,
  seriesGames,
  titleCell,
  era,
  INTERNATIONAL,
  WORLDS,
  boardCells,
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
    expect([...BUILT].sort()).toEqual(['overview', ...series].sort());
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

describe('the Hall of Fame overview', () => {
  const legend = (over: Partial<Legend>): Legend => ({
    name: 'ann',
    world_champion: false,
    world_titles: 0,
    international_titles: 0,
    intermezzo_titles: 0,
    royal_league_titles: 0,
    gold: 0,
    silver: 0,
    bronze: 0,
    ...over,
  });

  it('writes a title cell as a count, a crown for World titles, or a dash', () => {
    expect(titleCell(6)).toBe('6');
    expect(titleCell(1, true)).toBe('1 👑');
    expect(titleCell(0, true)).toBe('-');
  });

  it('marks a World Champion, or GM from three titles', () => {
    expect(legendMark(legend({ world_champion: true }))).toBe('WC');
    expect(legendMark(legend({ world_titles: 1 }))).toBe('WC');
    expect(legendMark(legend({ international_titles: 2, intermezzo_titles: 1 }))).toBe('GM');
    expect(legendMark(legend({ international_titles: 2 }))).toBeNull();
  });

  it('writes the medals and their total as upstream', () => {
    expect(majorMedals(legend({ gold: 15, silver: 6, bronze: 6 }))).toBe('🥇15  🥈6  🥉6 (27)');
  });

  it('sums the four major circuits’ games for the stat bar, as upstream', () => {
    const counts: SeriesCount[] = [
      { series: 'international', games: 31674, seasons: 34 },
      { series: 'intermezzo', games: 24872, seasons: 30 },
      { series: 'royal_league', games: 14740, seasons: 9 },
      { series: 'worlds', games: 31121, seasons: 5 },
      { series: 'grand_slams', games: 12868, seasons: 5222 },
    ];
    expect(derivedMatches(counts)).toBe('102,407+ Matches');
    expect(seriesGames({ series: 'grand_slams', games: 12868, seasons: 5222 })).toBe('12,868 games (5222 editions)');
  });
});

describe('boardCells', () => {
  const medals = { gold: 2, silver: 1, bronze: 0, games: null, wins: null, win_rate: null };
  const ladder = { gold: null, silver: null, bronze: null, games: 120, wins: 45, win_rate: 37.5 };

  it('gives a medal board its medal columns and counts', () => {
    expect(boardCells([medals])).toMatchObject({ headings: ['🥇 Gold', '🥈 Silver', '🥉 Bronze', 'Total'], rows: [['2', '1', '0', '3']] });
  });

  it('gives a ladder board its games, wins and win rate, not empty medal cells', () => {
    expect(boardCells([ladder])).toMatchObject({ headings: ['Games', 'Wins', 'Win %'], rows: [['120', '45', '37.5%']] });
  });

  it('writes thousands with separators', () => {
    expect(boardCells([{ ...ladder, games: 1234, wins: 1001 }]).rows[0]).toEqual(['1,234', '1,001', '37.5%']);
  });
});
