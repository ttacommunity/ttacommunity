import { describe, expect, it } from 'vitest';
import {
  divisionList,
  divisionText,
  hasPenalty,
  levelNamesPatch,
  penaltyValue,
  pointsList,
  pointsText,
  renamedFrom,
  scoringPatch,
  opensSettings,
  settingsHref,
  slugParam,
  toggledCounts,
} from './tournament-page';

describe('pointsList and pointsText', () => {
  it.each([
    ['dashes', '6-3-1-0', 4, [6, 3, 1, 0]],
    ['commas and spaces around', ' 5, 2, 0 ', 3, [5, 2, 0]],
    ['spaces', '2 0', 2, [2, 0]],
    ['a decimal', '2.5-1-0', 3, [2.5, 1, 0]],
  ])('read points per place from a box with %s', (_name, box, players, want) => {
    expect(pointsList(box, players)).toEqual(want);
  });

  it('read an empty box as no scoring for that player count', () => {
    expect(pointsList('  ', 3)).toEqual([]);
  });

  it('refuse points the API would refuse', () => {
    for (const [text, players] of [
      ['5-2', 3],
      ['2-5-0', 3],
      ['2-x', 2],
      ['3--1', 2],
      ['6-3-1-0', 3],
    ] as const) {
      expect(pointsList(text, players)).toBeNull();
    }
  });

  it('write points for the box pointsList reads', () => {
    expect(pointsText([6, 3, 1, 0])).toBe('6-3-1-0');
    expect(pointsText([])).toBe('');
  });
});

describe('levelNamesPatch', () => {
  const saved = { season_name: '', stage_name: 'Season' };

  it('sends only the names that changed, trimmed', () => {
    expect(levelNamesPatch({ season_name: ' Edition ', stage_name: 'Season' }, saved)).toEqual({ season_name: 'Edition' });
    expect(levelNamesPatch({ season_name: '', stage_name: '' }, saved)).toEqual({ stage_name: '' });
  });

  it('sends nothing when nothing changed, and refuses a name the API would refuse', () => {
    expect(levelNamesPatch({ season_name: ' ', stage_name: 'Season' }, saved)).toEqual({});
    expect(levelNamesPatch({ season_name: 'x'.repeat(33), stage_name: 'Season' }, saved)).toBeNull();
  });
});

describe('scoringPatch', () => {
  const saved = { points_2p: [], points_3p: [5, 2, 0], points_4p: [6, 3, 1, 0] };

  it('sends only the player counts whose points changed', () => {
    expect(scoringPatch({ 2: '', 3: '5-2-0', 4: '6-4-1-0' }, saved)).toEqual({ points_4p: [6, 4, 1, 0] });
    expect(scoringPatch({ 2: '4-0', 3: '', 4: '6-3-1-0' }, saved)).toEqual({ points_2p: [4, 0], points_3p: [] });
  });

  it('sends nothing when nothing changed, and refuses points the API would refuse', () => {
    expect(scoringPatch({ 2: '', 3: '5, 2, 0', 4: '6-3-1-0' }, saved)).toEqual({});
    expect(scoringPatch({ 2: '', 3: '5-2', 4: '6-3-1-0' }, saved)).toBeNull();
  });
});

describe('hasPenalty', () => {
  it.each([
    ['a numbered league season', 'league', '6', true],
    ['a named league season', 'league', '2023 Play Off', false],
    ['a cup season', 'cup', '2024', false],
  ])('offers a time-out penalty only for a league season with a number: %s', (_name, kind, season, want) => {
    expect(hasPenalty(kind, season)).toBe(want);
  });
});

describe('penaltyValue', () => {
  it.each([
    ['-1', -1],
    [' 0 ', 0],
    ['-0.5', -0.5],
  ])('reads a time-out penalty of "%s" as %d', (box, want) => {
    expect(penaltyValue(box)).toBe(want);
  });

  it('refuses a positive or unreadable penalty', () => {
    for (const text of ['1', '', 'minus one', '-']) {
      expect(penaltyValue(text)).toBeNull();
    }
  });
});

describe('settingsHref and slugParam', () => {
  it('link to a tournament settings page and read it back', () => {
    expect(settingsHref('royal_league')).toBe('/tournament/?t=royal_league');
    expect(slugParam(new URLSearchParams('t=royal_league'))).toBe('royal_league');
  });

  it('link to a group with its settings open, and read that back', () => {
    expect(settingsHref('quick_and_dirty', true)).toBe('/tournament/?t=quick_and_dirty&open=settings');
    expect(opensSettings(new URLSearchParams('t=quick_and_dirty&open=settings'))).toBe(true);
    expect(opensSettings(new URLSearchParams('t=quick_and_dirty'))).toBe(false);
  });

  it('refuse an address that names no tournament the API would accept', () => {
    for (const q of ['', 't=', 't=Worlds', 't=../me', 't=' + 'a'.repeat(65)]) {
      expect(slugParam(new URLSearchParams(q))).toBeNull();
    }
  });
});

describe('toggledCounts', () => {
  it('adds or removes a player count, kept in order', () => {
    expect(toggledCounts([2, 4], 3, true)).toEqual([2, 3, 4]);
    expect(toggledCounts([2, 3, 4], 2, false)).toEqual([3, 4]);
  });

  it('refuses to leave no player count rated (turn the tournament off instead)', () => {
    expect(toggledCounts([3], 3, false)).toBeNull();
  });
});

describe('divisionList and divisionText', () => {
  it('read top divisions from a comma-separated box, dropping blanks', () => {
    expect(divisionList(' Grandmaster, Diamond ,,')).toEqual(['Grandmaster', 'Diamond']);
    expect(divisionList('  ')).toEqual([]);
  });

  it('write them back for the box', () => {
    expect(divisionText(['Grandmaster', 'Diamond'])).toBe('Grandmaster, Diamond');
  });
});

describe('renamedFrom', () => {
  it.each([
    ['renamed', 'Season 2024', '2024', '2024'],
    ['kept', '2024', '2024', null],
    ['with no original', '2024', '', null],
  ])('gives the original name only when it was renamed: %s', (_name, name, original, want) => {
    expect(renamedFrom(name, original)).toBe(want);
  });
});
