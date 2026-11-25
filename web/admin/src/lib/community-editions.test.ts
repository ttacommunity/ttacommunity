import { describe, expect, it } from 'vitest';
import type { components } from '@ttac/shared/api';
import { editionNumber, newEdition } from './community-editions';

type Edition = components['schemas']['CommunityEditionItem'];

const edition = (slug: string, year: number, maintainer: string): Edition => ({
  slug, name: slug, title: slug, short_name: slug, badge: '', maintainer, maintainer_note: '',
  year, counting_events: 8, visible: true, sort_order: 1,
});

describe('newEdition', () => {
  it.each([
    ['the newest edition', [edition('e24', 2024, 'ann'), edition('e25', 2025, 'bob')], 'bob'],
    ['no edition yet', [], ''],
  ] as const)('takes its maintainer from %s', (_, editions, want) => {
    expect(newEdition(editions, 2026)).toMatchObject({ slug: '', year: 2026, maintainer: want });
  });
});

describe('editionNumber', () => {
  it.each([
    ['a year', 'year', '2026', 2026],
    ['an empty year', 'year', '', 0],
    ['counting events', 'counting_events', '6', 6],
    ['empty counting events, the default', 'counting_events', '', 8],
    ['no counting events, the default', 'counting_events', '0', 8],
    ['a sort order', 'sort_order', '3', 3],
    ['a sort order that is not a number', 'sort_order', 'x', 0],
  ] as const)('reads %s', (_, field, text, want) => {
    expect(editionNumber(field, text)).toBe(want);
  });
});
