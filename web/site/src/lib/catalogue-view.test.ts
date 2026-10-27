import { describe, expect, it } from 'vitest';
import { count } from '@ttac/shared/numbers';
import type { CatalogueGame } from '@ttac/shared/ratings';
import { catalogueGameRow, catalogueHref, cgeYears, entryRows, hasPodiums, gameStateLabel, gameWhere, outcomeNote, placeLabel, placeNames, plural, podiumCard, seasonName, shownName, spanYears, stageName, tournamentParam, seasonDates, activeYears, regulationsBox } from './catalogue-view';

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
  it.each([
    ['a numbered season', '24', undefined, 'Season 24'],
    ['a dated season', '2022 (Age A)', undefined, 'Season 2022 (Age A)'],
    ['a numbered season with its year', '1 (2024)', undefined, 'Season 1 (2024)'],
    ['a named season, as it is', 'First Edition (2023)', undefined, 'First Edition (2023)'],
    ['another named season, as it is', 'Grand Prix 2023', undefined, 'Grand Prix 2023'],
    ['a season the tournament calls an edition', '1 (2024)', 'Edition', 'Edition 1 (2024)'],
    ['a season the tournament calls a round', '31', 'Round', 'Round 31'],
    ['a named season of a tournament with editions', 'Grand Prix 2023', 'Edition', 'Grand Prix 2023'],
  ])('names %s', (_name, label, noun, want) => {
    expect(seasonName(label, noun)).toBe(want);
  });
});

describe('plural', () => {
  it.each([
    ['Season', 'Seasons'],
    ['Edition', 'Editions'],
    ['Stage', 'Stages'],
  ])('names more than one %s as %s', (noun, want) => {
    expect(plural(noun)).toBe(want);
  });
});

describe('placeLabel', () => {
  it('names the podium places as the Hall of Fame does', () => {
    expect([1, 2, 3].map(placeLabel)).toEqual(['🥇 Champion', '🥈 Runner-Up', '🥉 3rd Place']);
  });
});

describe('cgeYears', () => {
  it.each([
    ['started and finished', '2024-06-01', '2025-02-24', '2024 – 2025'],
    ['finished, no start recorded', null, '2023-05-23', '2023'],
    ['started, not finished', '2026-09-29', null, '2026 –'],
    ['neither', null, null, ''],
  ])('gives the years a CGE tournament ran: %s', (_name, started, finished, want) => {
    expect(cgeYears(started, finished)).toBe(want);
  });
});

describe('shownName', () => {
  it.each([
    ['not rated, marked', 'Nations Cup', false, 'Nations Cup (Not Rated)'],
    ['rated, as it is', 'Wimbledon 2024', true, 'Wimbledon 2024'],
  ])('names a tournament %s', (_name, name, rated, want) => {
    expect(shownName(name, rated)).toBe(want);
  });
});

describe('gameWhere', () => {
  it.each([
    ['all three', { stage: 'Stage 2', group: 'Lower 17', game: 5 }, 'Stage 2 · Lower 17 · game 5'],
    ['no stage', { stage: '', group: 'Bronze 3', game: 7 }, 'Bronze 3 · game 7'],
    ['a stage only', { stage: 'Stage 1', group: '', game: null }, 'Stage 1'],
    ['none of them', { stage: '', group: '', game: null }, ''],
  ])('names the stage, group and game a game sits in: %s', (_name, where, want) => {
    expect(gameWhere(where)).toBe(want);
  });
});

describe('outcomeNote', () => {
  it.each([
    ['resigned', 'resigned'],
    ['timed_out', 'timed out'],
    ['finished', ''],
    [null, ''],
  ])('notes an outcome of %s as "%s", marking only a seat that did not finish', (outcome, want) => {
    expect(outcomeNote(outcome)).toBe(want);
  });
});

describe('placeNames', () => {
  const at = (place: number, name: string, final = '') => ({ place, name, final, finished: '2025-01-01', source: '' });
  it('gives who took a place, naming the final when a season has several', () => {
    const podium = [at(1, 'ann', 'Odd Games'), at(2, 'bob', 'Odd Games'), at(1, 'cat', 'Even Games')];
    expect(placeNames(podium, 1)).toEqual([
      { name: 'ann', final: 'Odd Games' },
      { name: 'cat', final: 'Even Games' },
    ]);
    expect(placeNames(podium, 3)).toEqual([]);
  });
  it('leaves the final out when there is only one', () => {
    expect(placeNames([at(1, 'ann', 'Final')], 1)).toEqual([{ name: 'ann', final: '' }]);
  });
});

describe('podiumCard', () => {
  const at = (place: number, name: string, final = '') => ({ place, name, final, finished: '2025-01-01', source: '' });
  it("gives a recent podium's card: its season and the first name at each place", () => {
    const podium = [at(1, 'ann', 'Odd Games'), at(1, 'cat', 'Even Games'), at(2, 'bob', 'Odd Games')];
    expect(podiumCard({ slug: 'wimbledon', name: 'Wimbledon', season: '2024', season_name: 'Year', finished: '2025-02-24', podium })).toEqual({
      slug: 'wimbledon',
      name: 'Wimbledon',
      seasonLabel: '2024',
      seasonWord: 'Year',
      finished: '2025-02-24',
      champion: 'ann',
      runnerUp: 'bob',
      third: null,
    });
  });
});

describe('entryRows', () => {
  it("gives a tournament card's years, seasons, games and players", () => {
    expect(entryRows({ first_game: '2019-03-10', last_game: '2026-09-16', seasons: 31, games: 26352, players: 1900 })).toEqual([
      ['Active History', '2019 – 2026'],
      ['Seasons', '31'],
      ['Recorded Database Matches', count(26352)],
      ['Players', count(1900)],
    ]);
  });
});

describe('gameStateLabel', () => {
  it.each([
    ['a rated game, unmarked', 'played', true, ''],
    ['a game never played', 'never_played', false, 'Never played'],
    ['an unconfirmed result', 'unconfirmed', false, 'Unconfirmed result, not rated'],
    ['a played game its tournament, season or player count leaves unrated', 'played', false, 'Not rated'],
  ])('labels %s', (_name, state, rated, want) => {
    expect(gameStateLabel(state, rated)).toBe(want);
  });
});

describe('hasPodiums', () => {
  it('shows the podium columns when any season has a podium, a Grand Prix final among league seasons included', () => {
    expect(hasPodiums([{ podium: [] }, { podium: [{ place: 1 }] }])).toBe(true);
  });
  it("leaves them out when no season has one, as a ladder's seasons", () => {
    expect(hasPodiums([{ podium: [] }, { podium: [] }])).toBe(false);
    expect(hasPodiums([])).toBe(false);
  });
});

describe('seasonDates and activeYears', () => {
  const season = (first_game: string | null, last_game: string | null) => ({ first_game, last_game });

  it('show a season with no games as having none, and leave it out of the years', () => {
    expect(seasonDates(season('2024-07-01', '2025-02-24'))).toBe('2024-07-01 – 2025-02-24');
    expect(seasonDates(season(null, null))).toBe('No games recorded');
    expect(activeYears([season(null, null), season('2017-10-06', '2017-12-01'), season('2018-01-06', '2019-02-01')])).toBe('2017 – 2019');
    expect(activeYears([season(null, null)])).toBe('—');
  });
});

describe('regulationsBox', () => {
  const editions = [
    { cge_tournament: 4991, name: 'Wimbledon 2024', description: 'Best of three.\n\n  Finals live.  ', url: 'https://cge/4991' },
    { cge_tournament: 4369, name: 'Wimbledon 2023', description: 'CGE 2023.', url: 'https://cge/4369' },
  ];
  it("shows admin's text as bullets, one a line, with no CGE link, as DANeo's box", () => {
    expect(regulationsBox({ rules: editions, regulations: 'Groups of four.\nThe best 16 go on.\n' })).toEqual({
      lines: ['Groups of four.', 'The best 16 go on.'],
      source: null,
    });
  });
  it("shows the latest edition's CGE text as bullets when admin has written none, naming where it is from", () => {
    expect(regulationsBox({ rules: editions, regulations: null })).toEqual({
      lines: ['Best of three.', 'Finals live.'],
      source: { name: 'Wimbledon 2024', url: 'https://cge/4991' },
    });
  });
  it('shows nothing when neither has any', () => {
    expect(regulationsBox({ rules: [] })).toBeNull();
    expect(regulationsBox({ rules: [], regulations: ' \n ' })).toBeNull();
  });
});


describe('catalogueGameRow', () => {
  const game: CatalogueGame = {
    match_id: 7,
    date: '2026-05-31',
    season: '2026',
    stage: 'Stage 5',
    group: 'group 1',
    game: 1,
    state: 'played',
    rated: true,
    replay_code: 'COJOSAPOMA',
    seats: [
      { name: 'Ann', score: 235, outcome: 'finished' },
      { name: 'Cy', score: 0, outcome: 'resigned' },
    ],
  };
  it('turns a played game into a row: its season, where it was, each seat linked with its score and how it ended, the code', () => {
    expect(catalogueGameRow(game, 'Season')).toEqual({
      key: '7',
      date: '2026-05-31',
      context: 'Season 2026',
      where: 'Stage 5 · group 1 · game 1',
      tag: '',
      unplayed: false,
      seats: [
        { label: 'Ann', link: 'Ann', detail: '', score: 235, note: '' },
        { label: 'Cy', link: 'Cy', detail: '', score: 0, note: 'resigned' },
      ],
      code: 'COJOSAPOMA',
    });
  });
  it('marks a game never played, with its label', () => {
    const row = catalogueGameRow({ ...game, state: 'never_played', rated: false, replay_code: null }, 'Season');
    expect([row.unplayed, row.tag, row.code]).toEqual([true, 'Never played', null]);
  });
});
