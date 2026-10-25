import { describe, expect, it } from 'vitest';
import { ADMIN_EDITION_COLUMNS, ADMIN_ENTRANT_COLUMNS, editionFormatWarnings, editionHref, editionParams, formatAdvice, pointsChanged, pointsForm, pointsFrom, pointsSummary, settingsRows } from './blitz-page';
import { sortRows } from '@ttac/shared/sort';
import { formatLabel } from '@ttac/shared/blitz';
import { organisersSummary, threadTextSummary } from './blitz-threads';

describe('editionParams', () => {
  it.each([
    ['both names', '?t=us_open&e=2026', { slug: 'us_open', edition: '2026' }],
    ['an edition with spaces', '?t=us_open&e=Season+1', { slug: 'us_open', edition: 'Season 1' }],
    ['no edition', '?t=us_open', null],
    ['no tournament', '?e=2026', null],
    ['a slug that is not one', '?t=Bad!&e=2026', null],
    ['an empty edition', '?t=us_open&e=', null],
  ])('reads %s', (_name, search, want) => {
    expect(editionParams(new URLSearchParams(search))).toEqual(want);
  });
});

describe('editionHref', () => {
  it('escapes the edition', () => {
    expect(editionHref('us_open', 'Season 1')).toBe('/blitz/?t=us_open&e=Season%201');
  });
});

describe('formatAdvice', () => {
  it.each([
    ['the limit, half the field rounded down, and nothing to warn of', { swiss_rounds: 8, knockout_size: 8 }, 31, 15, []],
    ['Swiss rounds past the limit', { swiss_rounds: 8, knockout_size: 4 }, 12, 6, ['8 Swiss rounds is more than 12 active entrants can always be paired for: past 6, someone may have to meet an opponent again, and Pair refuses that. Late entrants or wildcards raise the limit.']],
    ['a knockout larger than the field', { swiss_rounds: 2, knockout_size: 8 }, 6, 3, ['The knockout takes the top 8, and 6 entrants are active.']],
    ['at least one round for a tiny field', { swiss_rounds: 1, knockout_size: 0 }, 1, 1, []],
  ])('gives %s', (_name, format, active, limit, warnings) => {
    expect(formatAdvice(format, active)).toEqual({ limit, warnings });
  });
});

describe('editionFormatWarnings', () => {
  const active = { status: 'active' };
  const field = [active, active, active, { status: 'dropped' }];
  it.each([
    ['nothing to warn of', { swiss_rounds: 1, knockout_size: 0 }, field, [{ kind: 'swiss' }], []],
    [
      'the saved format, for the active entrants only',
      { swiss_rounds: 8, knockout_size: 8 },
      field,
      [],
      [
        '8 Swiss rounds is more than 3 active entrants can always be paired for: past 1, someone may have to meet an opponent again, and Pair refuses that. Late entrants or wildcards raise the limit.',
        'The knockout takes the top 8, and 3 entrants are active.',
      ],
    ],
    ['none once the knockout has started', { swiss_rounds: 8, knockout_size: 8 }, field, [{ kind: 'swiss' }, { kind: 'knockout' }], []],
  ])('gives %s', (_name, format, entrants, rounds, want) => {
    expect(editionFormatWarnings({ format, entrants, rounds })).toEqual(want);
  });
});

const FIDE = { win: 1, draw: 0.5, loss: 0, forfeit_win: 1, forfeit_loss: 0, bye: 1, skip: 0 };

describe('pointsSummary', () => {
  it.each([
    ["FIDE's", FIDE, true, "FIDE's: win 1, draw ½, loss 0, forfeit win 1, forfeit loss 0, bye 1, skip 0"],
    ["the edition's own", { ...FIDE, win: 3, draw: 1, bye: 3 }, false, "This edition's: win 3, draw 1, loss 0, forfeit win 1, forfeit loss 0, bye 3, skip 0"],
  ])('%s', (_, points, fide, want) => {
    expect(pointsSummary(points, fide)).toBe(want);
  });
});

describe('pointsChanged', () => {
  it.each([
    ['nothing', FIDE, false],
    ['one outcome', { ...FIDE, skip: 0.5 }, true],
    ['another', { ...FIDE, forfeit_win: 0 }, true],
  ])('%s', (_, form, want) => {
    expect(pointsChanged(form, FIDE)).toBe(want);
  });
});

describe('pointsForm and pointsFrom', () => {
  it('fills the form with the points and reads it back', () => {
    expect(pointsFrom(pointsForm(FIDE))).toEqual(FIDE);
  });
  it('reads what was typed as numbers', () => {
    expect(pointsFrom({ ...pointsForm(FIDE), win: '2.5', draw: ' 1 ' })).toEqual({ ...FIDE, win: 2.5, draw: 1 });
  });
});

describe('settingsRows', () => {
  it('sums up the format, points, organisers and thread text, in that order', () => {
    const points = { win: 1, draw: 0.5, loss: 0, forfeit_win: 1, forfeit_loss: 0, bye: 1, skip: 0 };
    const ed = { format: { swiss_rounds: 8, knockout_size: 8 }, points, fide_points: true, organisers: [{ name: 'Carol' }], thread_text: { edited: true } };
    expect(settingsRows(ed)).toEqual([
      { key: 'Format', value: formatLabel(ed.format) },
      { key: 'Points', value: pointsSummary(points, true) },
      { key: 'Organisers', value: organisersSummary(ed.organisers) },
      { key: 'Thread text', value: threadTextSummary(ed.thread_text) },
    ]);
  });
});

describe('ADMIN_ENTRANT_COLUMNS', () => {
  const e = (name: string, discord: string, country: string | null, status: 'active' | 'dropped' | 'pending', skips: number[]) => ({ name, discord_name: discord, email: '', country, zone: 'UTC', utc_offset: '+00:00', rating: null, status, skips });
  const rows = [e('cy', 'zed', 'DE', 'active', [2]), e('Ann', '', null, 'dropped', []), e('bo', 'amy', 'CA', 'active', [2, 3])];
  it.each([
    ['Discord name, none last', { col: 'discord', desc: false }, ['bo', 'cy', 'Ann']],
    ['country, none last', { col: 'country', desc: false }, ['bo', 'cy', 'Ann']],
    ['status', { col: 'status', desc: false }, ['cy', 'bo', 'Ann']],
    ['skips, the most first', { col: 'skips', desc: true }, ['bo', 'cy', 'Ann']],
  ] as const)('sorts the entrants by %s', (_name, sort, want) => {
    expect(sortRows(rows, ADMIN_ENTRANT_COLUMNS, sort).map((r) => r.name)).toEqual(want);
  });
});

describe('ADMIN_EDITION_COLUMNS', () => {
  const ed = (name: string, dropped: number) => ({ name, edition: '2026', format: { swiss_rounds: 6, knockout_size: 8 }, round: null, champion: null, entrants: 10, dropped });
  it('sorts the editions by players dropped, the most first', () => {
    expect(sortRows([ed('a', 1), ed('b', 3)], ADMIN_EDITION_COLUMNS, { col: 'dropped', desc: true }).map((r) => r.name)).toEqual(['b', 'a']);
  });
});
