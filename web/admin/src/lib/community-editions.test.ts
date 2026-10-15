import { describe, expect, it } from 'vitest';
import type { components } from '@ttac/shared/api';
import { newEdition } from './community-editions';

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
