import { describe, expect, it } from 'vitest';
import { editionFormatWarnings, editionHref, editionParams, formatAdvice } from './blitz-page';

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
  const field = [active, active, active, { status: 'dropped' }, { status: 'pending' }];
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
