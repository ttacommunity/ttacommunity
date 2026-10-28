import { describe, expect, it } from 'vitest';
import { ADMIN_EDITION_COLUMNS, ADMIN_ENTRANT_COLUMNS, editionFormatWarnings, formatAdvice, pointsChanged, pointsForm, pointsFrom, pointsSummary, settingsRows, pairingFields } from './blitz-page';
import { sortRows } from '@ttac/shared/sort';
import { formatLabel, pairingSummary } from '@ttac/shared/blitz';

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
  it('sums up the format, points and pairing, in that order', () => {
    const points = { win: 1, draw: 0.5, loss: 0, forfeit_win: 1, forfeit_loss: 0, bye: 1, skip: 0 };
    const pairing = { first: 'dutch', later: 'rating', first_order: 'rating', later_order: 'rating' } as const;
    const ed = { format: { swiss_rounds: 8, knockout_size: 8 }, points, fide_points: true, pairing };
    expect(settingsRows(ed)).toEqual([
      { key: 'Format', value: formatLabel(ed.format) },
      { key: 'Points', value: pointsSummary(points, true) },
      { key: 'Pairing', value: pairingSummary(pairing) },
    ]);
  });
});

describe('ADMIN_ENTRANT_COLUMNS', () => {
  const e = (name: string, discord: string, country: string | null, status: 'active' | 'dropped' | 'pending', skips: number[], rank: number | null = null) => ({ name, discord_name: discord, email: '', country, zone: 'UTC', utc_offset: '+00:00', rating: null, starting_rank: rank, status, skips });
  const rows = [e('cy', 'zed', 'DE', 'active', [2], 2), e('Ann', '', null, 'dropped', []), e('bo', 'amy', 'CA', 'active', [2, 3], 1)];
  it.each([
    ['starting rank, none last', { col: 'rank', desc: false }, ['bo', 'cy', 'Ann']],
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

describe('pairingFields', () => {
  const p = (first: 'dutch' | 'timezone' | 'random', later: 'dutch' | 'rating' | 'random') => ({ first, later, first_order: 'rating' as const, later_order: 'lot' as const });
  it.each([
    ['both orders with more than one Swiss round and no Random', p('dutch', 'rating'), 6, null, { later: true, firstOrder: true, laterOrder: true, orders: ['rating', 'lot', 'random'] }],
    ["no order for a Random round 1", p('random', 'dutch'), 6, null, { later: true, firstOrder: false, laterOrder: true, orders: ['rating', 'lot', 'random'] }],
    ['no Later rounds or later order with a single Swiss round', p('timezone', 'dutch'), 1, null, { later: false, firstOrder: true, laterOrder: false, orders: ['rating', 'lot', 'random'] }],
    ['only the fixing order and random once round 1 fixed the ranks by lot', p('dutch', 'dutch'), 6, 'lot', { later: true, firstOrder: true, laterOrder: true, orders: ['lot', 'random'] }],
    ['only the fixing order and random once round 1 fixed the ranks by rating', p('timezone', 'random'), 6, 'rating', { later: true, firstOrder: true, laterOrder: false, orders: ['rating', 'random'] }],
  ] as const)('shows %s', (_, pairing, swiss, fixedBy, want) => {
    expect(pairingFields(pairing, swiss, fixedBy)).toEqual(want);
  });
});
