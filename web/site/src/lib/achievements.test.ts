import { describe, expect, it } from 'vitest';
import type { AchievementSummary, TournamentRecord } from '@ttac/shared/ratings';
import {
  achievementsHref,
  careerCells,
  honorRoll,
  matchesFilter,
  seasonCells,
  sortCareer,
  sortSeasons,
  titleBadge,
  recordCategory,
  cgeTournamentUrl,
  totalTitles,
} from './achievements';

const summary = (s: Partial<AchievementSummary>): AchievementSummary => ({
  world_titles: 0,
  international_titles: 0,
  intermezzo_titles: 0,
  royal_league_titles: 0,
  gold: 0,
  silver: 0,
  bronze: 0,
  accolades: [],
  ...s,
});

const rec = (r: Partial<TournamentRecord>): TournamentRecord => ({
  tournament: 'TCL Round 31',
  season: 'Tournament',
  division: 'Main',
  placement: '1 / 2',
  points: '80 pts',
  medal: null,
  details: 'Competed in Main',
  finish_date: '2024-01-02',
  career: false,
  ...r,
});

describe('honorRoll', () => {
  it('names each circuit title and the career medals, as upstream', () => {
    expect(
      honorRoll('Weidenbaum', summary({ world_titles: 1, international_titles: 6, intermezzo_titles: 8, gold: 15, silver: 6, bronze: 6 })),
    ).toBe(
      'Weidenbaum is a decorated competitor with 1x World Champion, 6x International Champion, 8x Intermezzo Champion (Total Career Medals: 15G / 6S / 6B).',
    );
    expect(honorRoll('saru', summary({ royal_league_titles: 2, gold: 2, silver: 2, bronze: 1 }))).toBe(
      'saru is a decorated competitor with 2x Royal League Champion (Total Career Medals: 2G / 2S / 1B).',
    );
  });

  it('counts medals alone for a player without a title', () => {
    expect(honorRoll('Veivi', summary({ silver: 1, bronze: 2 }))).toBe('Veivi has achieved 0G / 1S / 2B podium medals.');
  });
});

describe('totalTitles', () => {
  it('adds the four circuits', () => {
    expect(totalTitles(summary({ world_titles: 1, international_titles: 6, intermezzo_titles: 8, royal_league_titles: 2 }))).toBe(17);
    expect(totalTitles(null)).toBe(0);
  });
});

describe('titleBadge', () => {
  it('shows a World Champion for a current or past title, then the current title', () => {
    expect(titleBadge({ code: 'WC', count: 1, reason: '' }, { code: 'WC', year: 2025 })).toEqual({
      code: 'wc',
      text: 'World Champion',
      tooltip: 'Official Competitive Title: Reigning World Champion (2025)',
      reigning: true,
    });
    expect(titleBadge({ code: 'GM', count: 1, reason: '' }, { code: 'WC', year: 2023 })).toEqual({
      code: 'wc',
      text: 'World Champion',
      tooltip: 'Official Competitive Title: World Champion (2023)',
      reigning: false,
    });
    expect(titleBadge({ code: 'GM', count: 1, reason: '' }, { code: 'GM', year: 2024 })).toEqual({
      code: 'gm',
      text: 'Grandmaster',
      tooltip: 'Official Competitive Title: Grandmaster',
      reigning: false,
    });
    expect(titleBadge({ code: 'M', count: 1, reason: '' }, null)?.text).toBe('Master');
    expect(titleBadge(null, null)).toBeNull();
  });
});

describe('cells', () => {
  it('show a season row as its table does, a missing medal or points as a dash', () => {
    expect(seasonCells(rec({ medal: 'gold', points: '' }))).toEqual([
      '2024-01-02',
      'TCL Round 31',
      'Tournament',
      'Main',
      '⚡🥇 1 / 2',
      'Competed in Main',
    ]);
    expect(seasonCells(rec({ medal: null }))[4]).toBe('1 / 2');
    expect(careerCells(rec({ career: true, season: 'Career (2 seasons)', medal: null }))).toEqual([
      'TCL Round 31',
      'Career (2 seasons)',
      'Main',
      '1 / 2',
      '80 pts',
      '-',
      'Competed in Main',
    ]);
  });
});

describe('matchesFilter', () => {
  it("matches a row's text ignoring case, but not across two cells", () => {
    const r = seasonCells(rec({ medal: 'silver' }));
    expect(matchesFilter(r, 'COMPETED IN')).toBe(true);
    expect(matchesFilter(r, 'round 31')).toBe(true);
    expect(matchesFilter(r, '31 tournament')).toBe(false);
    expect(matchesFilter(r, '')).toBe(true);
  });
});

describe('sortSeasons', () => {
  const rows = [
    rec({ finish_date: '2024-01-02', placement: '10 / 12', tournament: 'b' }),
    rec({ finish_date: '2025-08-14', placement: '2 / 7', tournament: 'a' }),
    rec({ finish_date: '2023-05-01', placement: '1 / 4', tournament: 'C' }),
  ];

  it('orders placements by their first number, as upstream', () => {
    expect(sortSeasons(rows, 4, 'asc').map((r) => r.placement)).toEqual(['1 / 4', '2 / 7', '10 / 12']);
    expect(sortSeasons(rows, 4, 'desc').map((r) => r.placement)).toEqual(['10 / 12', '2 / 7', '1 / 4']);
  });

  it('orders the date and text columns as text, ignoring case', () => {
    expect(sortSeasons(rows, 0, 'asc').map((r) => r.finish_date)).toEqual(['2023-05-01', '2024-01-02', '2025-08-14']);
    expect(sortSeasons(rows, 1, 'asc').map((r) => r.tournament)).toEqual(['a', 'b', 'C']);
  });
});

describe('sortCareer', () => {
  it('orders numbers by value and text as text', () => {
    const rows = [
      rec({ career: true, points: '282 pts', tournament: 'International' }),
      rec({ career: true, points: '19 pts', tournament: 'Intermezzo' }),
      rec({ career: true, points: '5 Medals', tournament: 'Quick' }),
    ];
    expect(sortCareer(rows, 4, 'asc').map((r) => r.points)).toEqual(['5 Medals', '19 pts', '282 pts']);
    expect(sortCareer(rows, 0, 'desc').map((r) => r.tournament)).toEqual(['Quick', 'International', 'Intermezzo']);
  });
});

describe('achievementsHref', () => {
  it("links to a player's achievements page", () => {
    expect(achievementsHref('a b/c')).toBe('/ratings/player/achievements/?name=a+b%2Fc');
  });
});

describe('recordCategory', () => {
  it("files a tournament under upstream's medal styles", () => {
    expect(recordCategory('World Championship 2025')).toBe('wcs');
    expect(recordCategory('Intermezzo Championship')).toBe('major');
    expect(recordCategory('Royal League')).toBe('major');
    expect(recordCategory('Sodium Ladder')).toBe('ladder');
    expect(recordCategory('TCL Round 31')).toBe('ladder');
    expect(recordCategory('French Open 2026 (Odd Games) Stage 1')).toBe('slam');
    expect(recordCategory('Wimbledon 2025')).toBe('slam');
    expect(recordCategory('Survivors Cup')).toBe('other');
  });
});

describe('cgeTournamentUrl', () => {
  const page = (id: number) => `https://account.czechgames.com/tournaments/detail/${id}`;

  it("links a league to its CGE tournament, and a yearly event to that year's", () => {
    expect(cgeTournamentUrl('International Championship', 'Season 30')).toBe(page(21));
    expect(cgeTournamentUrl('World Championship', '2024')).toBe(page(4760));
    expect(cgeTournamentUrl('World Championship 2025', 'Championship')).toBe(page(5204));
    expect(cgeTournamentUrl('Wimbledon 2023', 'Stage 2')).toBe(page(4369));
    expect(cgeTournamentUrl('Leaderboard Trophy', '2025')).toBe(page(5695));
    expect(cgeTournamentUrl('Nations League', 'Season 7')).toBe(page(4127));
  });

  it('gives none where CGE has no page for it', () => {
    expect(cgeTournamentUrl('World Championship', '2021')).toBeNull();
    expect(cgeTournamentUrl('Quick and Dirty (Q&D)', 'Career Trophy Hall of Fame')).toBeNull();
  });
});
