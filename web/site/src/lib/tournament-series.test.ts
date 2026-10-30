import { describe, expect, it } from 'vitest';
import { count } from '@ttac/shared/numbers';
import type { Legend, SeriesCount } from '@ttac/shared/ratings';
import {
  BUILT,
  cultureScore,
  derivedMatches,
  legendMark,
  majorMedals,
  seriesGames,
  titleCell,
  CIRCUITS,
  circuitEra,
  circuitBox,
  circuitTitle,
  MAJORS,
  boardCells,
  leaguePoints,
  medalCells,
  opponentsLine,
  SERIES_NAV,
  seriesHref,
  seriesRecord,
  showsWorldChampion,
  worldChampions,
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

  it.each([
    ['international', '/ratings/tournaments/?t=international'],
    ['worlds', '/ratings/tournaments/?t=worlds'],
    ['grand_slams', '/ratings/hall_of_fame/grand_slams/'],
    ['overview', '/ratings/hall_of_fame/'],
  ] as const)('links %s to %s: a series to its page, the overview to the hub', (series, want) => {
    expect(seriesHref(series)).toBe(want);
  });
});

describe('the series figures', () => {
  it('writes points and scores as upstream does', () => {
    expect(leaguePoints(36)).toBe('36.0 pts');
    expect(leaguePoints(22.5)).toBe('22.5 pts');
    expect(cultureScore(388)).toBe('388 pts');
    expect(cultureScore(301.5)).toBe('302 pts');
  });

  it('lists opponents highest first, whole scores without a decimal', () => {
    expect(
      opponentsLine([
        { name: 'fulmar', score: 334 },
        { name: 'wren', score: 307.5 },
      ]),
    ).toBe('vs fulmar (334 pts), wren (307.5 pts)');
    expect(opponentsLine([])).toBe('');
  });

  // A league season shows its places; a Royal League season its wins, draws and losses.
  it.each([
    ['a 4-player league season', 'international', [6, 1, 0, 0], '6/1/0/0'],
    ['a 3-player league season', 'intermezzo', [6, 0, 0], '6/0/0'],
    ['a Royal League season without draws', 'royal_league', [6, 0, 1], '6/1'],
    ['a Royal League season with a draw', 'royal_league', [5, 1, 1], '5/1/1'],
  ] as const)('shows %s', (_name, series, record, want) => {
    expect(seriesRecord(series, [...record])).toBe(want);
  });

  it('has a page for each series built so far', () => {
    const series = ['international', 'intermezzo', 'royal_league', 'worlds', 'grand_slams', 'ladders'];
    expect([...MAJORS.map((m) => m.slug), ...CIRCUITS]).toEqual(series);
    expect([...BUILT].sort()).toEqual(['overview', ...series].sort());
  });

  it.each([
    ['from the year of its first game', '2017-09-25', '2017 – Present'],
    ['that it has none before its first game', null, 'No games yet'],
  ] as const)("states a circuit's era %s", (_name, firstGame, want) => {
    expect(circuitEra(firstGame)).toBe(want);
  });

  it.each([
    [
      "each tournament's format in brief, after its name",
      [{ slug: 'cup', name: 'Cup', format: 'Knockout.' }, { slug: 'open', name: 'Open' }, { slug: 'slam', name: 'Slam', format: 'Open.' }],
      { lines: ['Cup: Knockout.', 'Slam: Open.'], source: null },
    ],
    ['no box when none has a format in brief', [{ slug: 'open', name: 'Open', format: ' ' }], null],
  ] as const)("builds a circuit's format and regulations from %s", (_name, members, want) => {
    expect(circuitBox(members)).toEqual(want);
  });

  it("titles a circuit's page by its name", () => {
    expect(circuitTitle('Competitive Ladders')).toBe('Competitive Ladders | Tournaments Circuit');
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

  it('lists the World Champions from each final, latest first, with their other titles', () => {
    const seasons = [
      { label: '2023', in_champions: true, podium: [{ place: 2, name: 'bo' }, { place: 1, name: 'ann' }] },
      { label: '2024', in_champions: true, podium: [] },
      { label: '2025', in_champions: true, podium: [{ place: 1, name: 'cy' }] },
    ];
    const legends = [
      legend({ name: 'ann', world_titles: 1, international_titles: 6, intermezzo_titles: 1 }),
      legend({ name: 'cy', world_titles: 1, royal_league_titles: 2 }),
    ];
    expect(worldChampions(seasons, legends)).toEqual([
      { name: 'cy', season: '2025', icon: '👑', text: '2025 Reigning World Champion • 2x Royal League Champion' },
      { name: 'ann', season: '2023', icon: '🏆', text: '2023 World Champion • 6x International Champion • Intermezzo Champion' },
    ]);
  });

  it('leaves out a season admin keeps out of the champions, such as a play-off', () => {
    const seasons = [
      { label: '2023', in_champions: true, podium: [{ place: 1, name: 'ann' }] },
      { label: '2023 Play Off', in_champions: false, podium: [{ place: 1, name: 'dee' }] },
    ];
    expect(worldChampions(seasons, []).map((c) => c.name)).toEqual(['ann']);
  });

  it('lists no World Champions before any final has a winner', () => {
    expect(worldChampions([{ label: '2026', in_champions: true, podium: [] }], [])).toEqual([]);
  });

  it.each([
    ['a count', 6, false, '6'],
    ['a crown for World titles', 1, true, '1 👑'],
    ['a dash for none', 0, true, '-'],
  ])('writes a title cell as %s', (_name, titles, world, want) => {
    expect(titleCell(titles, world)).toBe(want);
  });

  it.each([
    ['the World Champion', { world_champion: true }, 'WC'],
    ['a past World Champion', { world_titles: 1 }, 'WC'],
    ['three titles', { international_titles: 2, intermezzo_titles: 1 }, 'GM'],
    ['two titles', { international_titles: 2 }, null],
  ])('marks %s', (_name, fields, want) => {
    expect(legendMark(legend(fields))).toBe(want);
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
    expect(derivedMatches(counts)).toBe(`${count(102407)}+ Matches`);
    expect(seriesGames({ series: 'grand_slams', games: 12868, seasons: 5222 })).toBe(`${count(12868)} games (5222 editions)`);
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
    expect(boardCells([{ ...ladder, games: 1234, wins: 1001 }]).rows[0]).toEqual([count(1234), count(1001), '37.5%']);
  });
});

describe('showsWorldChampion', () => {
  it.each([
    ['the World Champion on the World Championship', 'worlds', true, true],
    ['another player on the World Championship', 'worlds', false, false],
    ['the World Champion on the International', 'international', true, false],
    ['the World Champion on the Royal League', 'royal_league', true, false],
  ] as const)('marks %s', (_name, series, champion, want) => {
    expect(showsWorldChampion(series, champion)).toBe(want);
  });
});
