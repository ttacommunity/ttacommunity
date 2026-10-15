/** The community leaderboard's editions in admin: what a new edition starts from. */
import type { components } from '@ttac/shared/api';

type Edition = components['schemas']['CommunityEditionItem'];

/** newEdition returns a blank, shown edition for year, with the newest of editions' maintainer ("" with none). */
export function newEdition(editions: readonly Edition[], year: number): Edition {
  const newest = editions.reduce<Edition | undefined>((a, e) => (a === undefined || e.year > a.year ? e : a), undefined);
  return {
    slug: '',
    name: '',
    title: '',
    short_name: '',
    badge: '⭐ Community',
    maintainer: newest?.maintainer ?? '',
    maintainer_note: 'Tracking tournament finishes under the official Best-8 system.',
    year,
    counting_events: 8,
    visible: true,
    sort_order: 10,
  };
}
