import { describe, expect, it } from 'vitest';
import type { AchievementSummary, TournamentRecord } from '@ttac/shared/ratings';
import {
  achievementsHref,
  bestResultsPerTournament,
  canonicalTournamentName,
  careerCells,
  careerResult,
  cgeTournamentUrl,
  formatTournamentDisplay,
  honorRoll,
  isPromotion,
  isQnD,
  majorBestResults,
  matchesFilter,
  matchesTournamentFilter,
  recordCategory,
  seasonCells,
  seasonDisplay,
  sortCareer,
  sortSeasons,
  sortTournaments,
  titleBadge,
  totalTitles,
  tournamentCategory,
  tournamentOverviews,
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
      honorRoll('Willow_Tree', summary({ world_titles: 1, international_titles: 6, intermezzo_titles: 8, gold: 15, silver: 6, bronze: 6 })),
    ).toBe(
      'Willow_Tree is a decorated competitor with 1x World Champion, 6x International Champion, 8x Intermezzo Champion (Total Career Medals: 15G / 6S / 6B).',
    );
    expect(honorRoll('sora', summary({ royal_league_titles: 2, gold: 2, silver: 2, bronze: 1 }))).toBe(
      'sora is a decorated competitor with 2x Royal League Champion (Total Career Medals: 2G / 2S / 1B).',
    );
  });

  it('counts medals alone for a player without a title', () => {
    expect(honorRoll('Vireo', summary({ silver: 1, bronze: 2 }))).toBe('Vireo has achieved 0G / 1S / 2B podium medals.');
  });
});

describe('totalTitles', () => {
  it('adds the four circuits', () => {
    expect(totalTitles(summary({ world_titles: 1, international_titles: 6, intermezzo_titles: 8, royal_league_titles: 2 }))).toBe(17);
    expect(totalTitles(null)).toBe(0);
  });
});

describe('titleBadge', () => {
  it.each([
    [
      'the reigning World Champion',
      'WC',
      { code: 'WC', year: 2025 },
      { code: 'wc', text: 'World Champion', tooltip: 'Official Competitive Title: Reigning World Champion (2025)', reigning: true },
    ],
    [
      'a past World Champion over the current title',
      'GM',
      { code: 'WC', year: 2023 },
      { code: 'wc', text: 'World Champion', tooltip: 'Official Competitive Title: World Champion (2023)', reigning: false },
    ],
    [
      'the current title when the peak is no higher',
      'GM',
      { code: 'GM', year: 2024 },
      { code: 'gm', text: 'Grandmaster', tooltip: 'Official Competitive Title: Grandmaster', reigning: false },
    ],
  ] as const)('shows %s', (_name, current, peak, want) => {
    expect(titleBadge({ code: current, count: 1, reason: '' }, peak)).toEqual(want);
  });

  it('shows the current title with no peak, and nothing with neither', () => {
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
    expect(careerCells(rec({ career: true, season: 'Career (2 seasons)', medal: null, placement: '1 / 2' }))).toEqual([
      'TCL Round 31',
      'Career (2 seasons)',
      'Main',
      '1 / 2',
      'Competed in Main',
    ]);
  });
});

describe('careerResult', () => {
  it.each([
    ['1x 1st, 0x 2nd, 0x 3rd', '🥇 1'],
    ['0x 1st, 1x 2nd, 2x 3rd', '🥈 1   🥉 2'],
    ['2 Titles, 1 Runner-ups', '🥇 2   🥈 1'],
    ['3 Championships', '🥇 3'],
    ['4 / 8', '4 / 8'],
  ])('formats %s as %s', (placement, want) => {
    expect(careerResult(rec({ placement, medal: null }))).toBe(want);
  });
});

describe('matchesFilter', () => {
  const r = seasonCells(rec({ medal: 'silver' }));

  it.each([
    ['text in another case', 'COMPETED IN', true],
    ['part of a cell', 'round 31', true],
    ['text across two cells', '31 tournament', false],
    ['no filter', '', true],
  ])("matches a row's text ignoring case, but not across two cells: %s", (_name, filter, want) => {
    expect(matchesFilter(r, filter)).toBe(want);
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
  it('orders text and results cleanly', () => {
    const rows = [
      rec({ career: true, tournament: 'International', placement: '1x 1st, 0x 2nd, 0x 3rd', medal: 'gold' }),
      rec({ career: true, tournament: 'Intermezzo', placement: '0x 1st, 1x 2nd, 0x 3rd', medal: 'silver' }),
      rec({ career: true, tournament: 'Royal League', placement: '2x 1st, 0x 2nd, 0x 3rd', medal: 'gold' }),
    ];
    expect(sortCareer(rows, 0, 'asc').map((r) => r.tournament)).toEqual(['Intermezzo', 'International', 'Royal League']);
    expect(sortCareer(rows, 3, 'desc').map((r) => r.tournament)).toEqual(['Royal League', 'International', 'Intermezzo']);
  });
});

describe('achievementsHref', () => {
  it("links to a player's achievements page", () => {
    expect(achievementsHref('a b/c')).toBe('/ratings/player/achievements/?name=a+b%2Fc');
  });
});

describe('recordCategory', () => {
  it.each([
    ['World Championship 2025', 'wcs'],
    ['Intermezzo Championship', 'major'],
    ['Royal League', 'major'],
    ['Sodium Ladder', 'ladder'],
    ['TCL Round 31', 'ladder'],
    ['French Open 2026 (Odd Games) Stage 1', 'slam'],
    ['Wimbledon 2025', 'slam'],
    ['Survivors Cup', 'other'],
  ])("files %s under upstream's medal style %s", (tournament, want) => {
    expect(recordCategory(tournament)).toBe(want);
  });
});

describe('cgeTournamentUrl', () => {
  const page = (id: number) => `https://account.czechgames.com/tournaments/detail/${id}`;

  // A league links to its CGE tournament, a yearly event to that year's; none where CGE has no page.
  it.each([
    ['International Championship', 'Season 30', page(21)],
    ['World Championship', '2024', page(4760)],
    ['World Championship 2025', 'Championship', page(5204)],
    ['Wimbledon 2023', 'Stage 2', page(4369)],
    ['Leaderboard Trophy', '2025', page(5695)],
    ['Nations League', 'Season 7', page(4127)],
    ['Slow Burn 1 (2024)', 'Season 10', page(4807)],
    ['Slow Burn 2 (2026)', 'Season 1', page(5979)],
    ['World Championship', '2021', null],
    ['Quick and Dirty (Q&D)', 'Career Trophy Hall of Fame', null],
  ])('links %s, %s to %s', (tournament, season, want) => {
    expect(cgeTournamentUrl(tournament, season)).toBe(want);
  });
});

describe('formatTournamentDisplay', () => {
  it.each([
    ['RL_tb_s09', 'Royal League Season 9 Tiebreaks'],
    ['RL_tb_s01', 'Royal League Season 1 Tiebreaks'],
    ['RL_tb_03', 'Royal League Season 3 Tiebreaks'],
    ['RL_2w_tb_s02', 'Royal League Season 2 2-Way Tiebreak'],
    ['RL_3w_tb_s02 Duke', 'Royal League Season 2 3-Way Tiebreak (Duke)'],
    ['RL_s07 - Emperor game 12', 'Royal League Season 7 - Emperor Game 12'],
    ['RL_s06 - King game 1', 'Royal League Season 6 - King Game 1'],
    ['RL_s02', 'Royal League Season 2'],
    ['Worlds 2 - stage 4', 'World Championship 2 - Stage 4'],
    ['Worlds 2024 stage 3', 'World Championship 2024 - Stage 3'],
    ['NL_s07', 'Nations League Season 7'],
    ['IC_s05', 'International Championship Season 5'],
    ['IM_s04', 'Intermezzo Season 4'],
    ['TCL Round 85', 'Transcontinental Ladder Round 85'],
  ])('formats %s -> %s', (raw, want) => {
    expect(formatTournamentDisplay(raw)).toBe(want);
  });
});

describe('canonicalTournamentName', () => {
  it.each([
    ['World Championship 2024', 'World Championship'],
    ['World Championship 2', 'World Championship'],
    ['Worlds 1', 'World Championship'],
    ['Royal League', 'Royal League'],
    ['RL_tb_s09', 'Royal League'],
    ['RL_3w_tb_s08 Duke', 'Royal League'],
    ['RL_s07', 'Royal League'],
    ['Quick and Dirty Stage 91', 'Quick and Dirty'],
    ['Q&D 2p Stage 12', 'Quick and Dirty'],
    ['Quick and Dirty (Q&D)', 'Quick and Dirty'],
    ['International S02', 'International Championship'],
    ['Intermezzo S01', 'Intermezzo Championship'],
  ])('maps %s -> %s', (raw, want) => {
    expect(canonicalTournamentName(raw)).toBe(want);
  });
});

describe('seasonDisplay', () => {
  it('formats season displays consistently', () => {
    expect(seasonDisplay(rec({ tournament: 'World Championship 2024', season: 'Championship' }))).toBe('2024');
    expect(seasonDisplay(rec({ tournament: 'World Championship 2', season: 'Stage 1' }))).toBe('Edition 2 (Stage 1)');
    expect(seasonDisplay(rec({ tournament: 'RL_tb_s09', season: 'Season 9 Tiebreaks' }))).toBe('Season 9 Tiebreaks');
    expect(seasonDisplay(rec({ tournament: 'Quick and Dirty Stage 91', season: 'Season 31', division: 'group 1' }))).toBe('Season 31 (Stage 91)');
  });
});

describe('isQnD', () => {
  it('detects all Q&D variants', () => {
    expect(isQnD('Quick and Dirty Stage 91')).toBe(true);
    expect(isQnD('Q&D 2p')).toBe(true);
    expect(isQnD('Royal League')).toBe(false);
  });
});

describe('tournamentCategory', () => {
  it.each([
    ['World Championship 2025', 'major'],
    ['Royal League', 'major'],
    ['RL_s07', 'major'],
    ['International Championship', 'major'],
    ['Intermezzo', 'major'],
    ['Wimbledon 2025', 'slam'],
    ['French Open 2025', 'slam'],
    ['Australian Open 2026', 'slam'],
    ['Survivors Cup', 'slam'],
    ['Leaderboard Trophy', 'slam'],
    ['Nations Cup', 'slam'],
    ['Transcontinental Ladder', 'ladder'],
    ['TCL Round 85', 'ladder'],
    ['Sodium Ladder', 'ladder'],
    ['Eiffel Tower', 'ladder'],
    ['Nations League', 'ladder'],
    ['Premier League', 'ladder'],
    ['Random Community Tournament', 'community'],
  ])('categorizes %s as %s', (tournament, want) => {
    expect(tournamentCategory(tournament)).toBe(want);
  });
});

describe('isPromotion', () => {
  it('identifies 1st place finishes in lower divisions as promotions', () => {
    expect(isPromotion(rec({ tournament: 'Royal League', division: 'King', placement: '1 / 8' }))).toBe(true);
    expect(isPromotion(rec({ tournament: 'Royal League', division: 'Archduke', placement: '1 / 8' }))).toBe(true);
    expect(isPromotion(rec({ tournament: 'International', division: 'Tier 2', placement: '1 / 4' }))).toBe(true);
  });

  it('rejects top division wins or non-1st placements', () => {
    expect(isPromotion(rec({ tournament: 'Royal League', division: 'Emperor', placement: '1 / 8' }))).toBe(false);
    expect(isPromotion(rec({ tournament: 'Royal League', division: 'King', placement: '2 / 8' }))).toBe(false);
    expect(isPromotion(rec({ tournament: 'Royal League', division: 'Archduke', placement: '3 / 8' }))).toBe(false);
  });

  it('never treats Q&D wins as promotions', () => {
    expect(isPromotion(rec({ tournament: 'Quick and Dirty', division: 'group 1', placement: '1 / 4' }))).toBe(false);
    expect(isPromotion(rec({ tournament: 'Quick and Dirty Stage 91', division: 'group 2', placement: '1 / 4' }))).toBe(false);
    expect(isPromotion(rec({ tournament: 'Q&D 2p Stage 12', division: 'group 3', placement: '1 / 2' }))).toBe(false);
  });
});

describe('bestResultsPerTournament', () => {
  it('picks the single best result per tournament series', () => {
    const records = [
      rec({ tournament: 'Royal League', season: 'Season 1', division: 'Baron', placement: '1 / 8' }),
      rec({ tournament: 'Royal League', season: 'Season 4', division: 'Emperor', placement: '2 / 8', medal: 'silver' }),
      rec({ tournament: 'Royal League', season: 'Season 9', division: 'Emperor', placement: '1 / 8', medal: 'gold' }),
      rec({ tournament: 'Wimbledon 2024', season: '2024', division: 'Main', placement: '9 / 128' }),
      rec({ tournament: 'Wimbledon 2025', season: '2025', division: 'Main', placement: '1 / 256', medal: 'gold' }),
    ];
    const best = bestResultsPerTournament(records);
    expect(best).toHaveLength(2);
    expect(best.find((b) => b.tournament.includes('Royal League'))?.placement).toBe('1 / 8');
    expect(best.find((b) => b.tournament.includes('Royal League'))?.medal).toBe('gold');
    expect(best.find((b) => b.tournament.includes('Wimbledon'))?.medal).toBe('gold');
  });
});

describe('majorBestResults', () => {
  it('extracts peak divisions and parses total matches and win rate', () => {
    const records = [
      rec({
        tournament: 'Royal League',
        season: 'Season 6',
        division: 'King',
        placement: '1 / 8',
        details: 'Competed in King: 7 matches played, 6 victories (85.7% win rate)',
      }),
      rec({
        tournament: 'International Championship',
        season: 'Season 25',
        division: 'Division 2',
        placement: '2 / 4',
        details: 'Competed in Division 2: 6 matches played, 3 victories (50.0% win rate)',
      }),
    ];
    const res = majorBestResults(records);
    expect(res.rl?.division).toBe('King');
    expect(res.ic?.division).toBe('Division 2');
    expect(res.im).toBeNull();
    expect(res.totalMatches).toBe(13);
    expect(res.winRate).toBeCloseTo(69.2, 1);
  });
});

describe('tournamentOverviews', () => {
  it('aggregates multiple seasons into one entry per tournament with cumulative matches', () => {
    const records = [
      rec({
        tournament: 'Royal League',
        season: 'Season 8',
        division: 'King',
        placement: '1 / 8',
        details: 'Competed in King: 7 matches played, 6 victories (85.7% win rate)',
      }),
      rec({
        tournament: 'Royal League',
        season: 'Season 9',
        division: 'Emperor',
        placement: '1 / 8',
        medal: 'gold',
        details: 'Competed in Emperor: 7 matches played, 7 victories (100.0% win rate)',
      }),
      rec({
        tournament: 'RL_tb_s09',
        season: 'Season 9 Tiebreaks',
        division: 'Tiebreak',
        placement: '1 / 2',
        details: 'Competed in Tiebreak: 1 matches played, 1 victories (100.0% win rate)',
      }),
      rec({
        tournament: 'World Championship 2024',
        season: 'Championship',
        division: 'Championship',
        placement: '47 / 1120',
        details: 'Competed in Championship: 32 matches played, 21 victories (65.6% win rate)',
      }),
      rec({
        tournament: 'World Championship 2',
        season: 'Stage 1',
        division: 'stage 1',
        placement: '1 / 800',
        details: 'Competed in stage 1: 8 matches played, 8 victories (100.0% win rate)',
      }),
      rec({
        tournament: 'Quick and Dirty Stage 90',
        season: 'Stage 90',
        division: 'group 1',
        placement: '2 / 3',
        details: 'Competed in Main: 1 matches played, 0 victories (0.0% win rate)',
      }),
      rec({
        tournament: 'Quick and Dirty Stage 91',
        season: 'Stage 91',
        division: 'group 2',
        placement: '3 / 4',
        details: 'Competed in Main: 1 matches played, 0 victories (0.0% win rate)',
      }),
    ];
    const overviews = tournamentOverviews(records);
    expect(overviews).toHaveLength(3);

    const rl = overviews.find((o) => o.tournament === 'Royal League');
    expect(rl).toBeDefined();
    expect(rl?.seasonsCount).toBe(3);
    expect(rl?.totalMatches).toBe(15);
    expect(rl?.wins).toBe(14);
    expect(rl?.losses).toBe(1);
    expect(rl?.bestRecord.division).toBe('Emperor');
    expect(rl?.bestRecord.medal).toBe('gold');

    const wc = overviews.find((o) => o.tournament === 'World Championship');
    expect(wc).toBeDefined();
    expect(wc?.seasonsCount).toBe(2);
    expect(wc?.totalMatches).toBe(40);
    expect(wc?.wins).toBe(29);
    expect(wc?.losses).toBe(11);

    const qd = overviews.find((o) => o.tournament === 'Quick and Dirty');
    expect(qd).toBeDefined();
    expect(qd?.seasonsCount).toBe(2);
    expect(qd?.totalMatches).toBe(2);
    expect(qd?.wins).toBe(0);
    expect(qd?.losses).toBe(2);
  });
});

describe('sortTournaments', () => {
  it('sorts by seasons count, match record, or tournament name', () => {
    const list = tournamentOverviews([
      rec({ tournament: 'Alpha', season: 'Season 1', details: '10 matches played, 8 victories' }),
      rec({ tournament: 'Beta', season: 'Season 1', details: '5 matches played, 1 victories' }),
      rec({ tournament: 'Beta', season: 'Season 2', details: '5 matches played, 2 victories' }),
    ]);
    const bySeasonsDesc = sortTournaments(list, 1, 'desc');
    expect(bySeasonsDesc[0].tournament).toBe('Beta');

    const byNameAsc = sortTournaments(list, 0, 'asc');
    expect(byNameAsc[0].tournament).toBe('Alpha');
  });
});

describe('matchesTournamentFilter', () => {
  it('matches by tournament name, display name, division, or placement', () => {
    const list = tournamentOverviews([
      rec({ tournament: 'Royal League', division: 'Emperor', placement: '1 / 8' }),
    ]);
    const t = list[0];
    expect(matchesTournamentFilter(t, 'Royal')).toBe(true);
    expect(matchesTournamentFilter(t, 'Emperor')).toBe(true);
    expect(matchesTournamentFilter(t, '1 / 8')).toBe(true);
    expect(matchesTournamentFilter(t, 'Nonexistent')).toBe(false);
  });
});
