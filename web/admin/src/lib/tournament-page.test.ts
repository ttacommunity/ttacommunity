import { describe, expect, it } from 'vitest';
import {
  divisionList,
  divisionsChanged,
  divisionText,
  fieldId,
  hasPenalty,
  levelNamesPatch,
  noteAction,
  patchReady,
  penaltySavable,
  penaltyValue,
  pointsList,
  pointsText,
  renamedFrom,
  scoringPatch,
  opensSettings,
  settingsHref,
  slugParam,
  toggled,
  toggledCounts,
  tournamentLoad,
} from './tournament-page';
import type { AdminResult } from './admin-api';

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

describe('divisionsChanged', () => {
  it.each([
    ['the same but for spaces and blanks', ' Grandmaster ,Diamond,', false],
    ['one more', 'Grandmaster, Diamond, Gold', true],
    ['emptied', ' ', true],
  ])('sees top divisions as changed when %s', (_name, text, want) => {
    expect(divisionsChanged(text, ['Grandmaster', 'Diamond'])).toBe(want);
  });
});

describe('patchReady', () => {
  it.each([
    ['a change', { season_name: 'Edition' }, true],
    ['nothing changed', {}, false],
    ['a refused box', null, false],
  ])('lets a patch be saved with %s', (_name, patch, want) => {
    expect(patchReady(patch)).toBe(want);
  });
});

describe('penaltySavable', () => {
  it.each([
    ['a new penalty', -2, -1, true],
    ['the saved one', -1, -1, false],
    ['an unreadable box', null, -1, false],
  ])('lets a penalty be saved with %s', (_name, value, saved, want) => {
    expect(penaltySavable(value, saved)).toBe(want);
  });
});

describe('fieldId', () => {
  it.each([
    ['plain parts', ['note', 'royal_league', '6', ''], 'note-royal_league-6-'],
    ['a season label with spaces and a slash', ['season-rated', '2023 Play Off/A'], 'season-rated-2023_Play_Off_A'],
  ])('builds an element id from %s', (_name, parts, want) => {
    expect(fieldId(...parts)).toBe(want);
  });
});

describe('toggled', () => {
  it.each([
    ['adds one not there', ['1'], '2', ['1', '2']],
    ['removes one there', ['1', '2'], '1', ['2']],
  ])('%s', (_name, list, item, want) => {
    expect(toggled(list, item)).toEqual(want);
  });
});

describe('noteAction', () => {
  it.each([
    ['an open note closes', true, 'Best of three', 'close'],
    ['a closed empty note is added', false, '', 'add'],
    ['a closed note is edited', false, 'Best of three', 'edit'],
  ] as const)('%s', (_name, open, note, want) => {
    expect(noteAction(open, note)).toBe(want);
  });
});

describe('tournamentLoad', () => {
  const T = { slug: 'royal_league' } as never;
  const ok = <D>(data: D): AdminResult<D> => ({ ok: true, data });
  const refused = (status: number, message: string): AdminResult<never> => ({ ok: false, status, message });

  it.each([
    ['the tournament refused', refused(500, 'The API is down.'), ok([]), 'The API is down.'],
    ['the list refused', ok(T), refused(500, 'The list is down.'), 'The list is down.'],
  ])('passes the API\'s reason on when %s', (_name, one, rows, want) => {
    expect(tournamentLoad('royal_league', one, rows)).toEqual({ kind: 'error', message: want });
  });

  it('names the tournament the API does not know', () => {
    const got = tournamentLoad('royal_league', refused(404, 'Not found.'), ok([]));
    expect(got.kind === 'error' && got.message.includes('royal_league')).toBe(true);
  });

  it('is ready when both answers are', () => {
    expect(tournamentLoad('royal_league', ok(T), ok([]))).toEqual({ kind: 'ready', data: T });
  });
});
