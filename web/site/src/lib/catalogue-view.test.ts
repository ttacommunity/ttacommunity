import { describe, expect, it } from 'vitest';
import { catalogueHref, cgeYears, championsFromFinals, entryRows, gameStateLabel, gameWhere, outcomeNote, placeLabel, placeNames, seasonName, winsLabel, shownName, spanYears, stageName, tournamentParam, seasonDates, activeYears, regulationsBox } from './catalogue-view';

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

describe('winsLabel', () => {
  it('counts a ladder winner\'s top-tier wins only when there is more than one', () => {
    expect(winsLabel(10)).toBe('10 wins');
    expect(winsLabel(3.5)).toBe('3.5 wins');
    expect(winsLabel(1.5)).toBe('1.5 wins');
    expect(winsLabel(1)).toBe('');
  });
});

describe('entryRows', () => {
  it("gives a tournament card's years, seasons, games and players", () => {
    expect(entryRows({ first_game: '2019-03-10', last_game: '2026-09-16', seasons: 31, games: 26352, players: 1900 })).toEqual([
      ['Active History', '2019 – 2026'],
      ['Seasons', '31'],
      ['Recorded Database Matches', '26,352'],
      ['Players', '1,900'],
    ]);
  });
});

describe('gameStateLabel', () => {
  it('marks a game left out of the ratings', () => {
    expect(gameStateLabel('played')).toBe('');
    expect(gameStateLabel('never_played')).toBe('Never played');
    expect(gameStateLabel('unconfirmed')).toBe('Unconfirmed result, not rated');
  });
});

describe('championsFromFinals', () => {
  const season = (label: string, champion: string | null) => ({ label, podium: champion === null ? [] : [{ place: 1, name: champion }] });
  it("gives a tournament crowned by its finals only the seasons that had one", () => {
    const seasons = [season('2', null), season('Grand Prix 2023', 'FirstOfDao'), season('3', null)];
    expect(championsFromFinals('duel_curve', seasons)?.map((s) => s.label)).toEqual(['Grand Prix 2023']);
  });
  it('leaves other tournaments to their seasons', () => {
    expect(championsFromFinals('intermezzo', [season('24', 'pv4')])).toBeNull();
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
    { cge_tournament: 4991, name: 'Wimbledon 2024', description: 'CGE 2024.', url: 'https://cge/4991' },
    { cge_tournament: 4369, name: 'Wimbledon 2023', description: 'CGE 2023.', url: 'https://cge/4369' },
  ];
  it("shows admin's text over CGE's, still linking the latest edition", () => {
    expect(regulationsBox({ rules: editions, regulations: 'Best of three.' })).toEqual({
      text: 'Best of three.',
      link: { name: 'Wimbledon 2024', url: 'https://cge/4991' },
    });
  });
  it("shows the latest edition's CGE text when admin has written none", () => {
    expect(regulationsBox({ rules: editions })).toEqual({ text: 'CGE 2024.', link: { name: 'Wimbledon 2024', url: 'https://cge/4991' } });
  });
  it("shows admin's text with no link when CGE has none, and nothing when neither has", () => {
    expect(regulationsBox({ rules: [], regulations: 'Ours.' })).toEqual({ text: 'Ours.', link: null });
    expect(regulationsBox({ rules: [] })).toBeNull();
  });
});
