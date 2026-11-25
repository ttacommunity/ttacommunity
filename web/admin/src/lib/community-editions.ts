/** The community leaderboard's editions in admin: what a new edition starts from, and how its number fields read what is typed. */
import type { components } from '@ttac/shared/api';

type Edition = components['schemas']['CommunityEditionItem'];

/** EditionFieldValues are the fields an edition's forms edit, besides its slug and visibility. */
export type EditionFieldValues = Pick<Edition, 'name' | 'short_name' | 'title' | 'badge' | 'maintainer' | 'maintainer_note' | 'year' | 'counting_events' | 'sort_order'>;

/** NumberField is an edition field typed as a whole number. */
export type NumberField = 'year' | 'counting_events' | 'sort_order';

const COUNTING_EVENTS = 8;
const FALLBACK: Record<NumberField, number> = { year: 0, counting_events: COUNTING_EVENTS, sort_order: 0 };

/** editionNumber reads text typed into field as a whole number, or the field's fallback when it is empty, not a number or 0. */
export function editionNumber(field: NumberField, text: string): number {
  return parseInt(text, 10) || FALLBACK[field];
}

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
    counting_events: COUNTING_EVENTS,
    visible: true,
    sort_order: 10,
  };
}
