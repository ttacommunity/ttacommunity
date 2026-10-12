/** The blitz edition page's decisions: the edition the address names, and the players' view. */
import type { components } from '@ttac/shared/api';

type Schemas = components['schemas'];
export type AdminBlitzEntrant = Schemas['AdminBlitzEntrant'];

// The API's pattern for a tournament in a path.
const SLUG = /^[a-z][a-z0-9_]{0,63}$/;

/** editionParams returns the tournament slug (?t=) and edition label (?e=) the address names, or null. */
export function editionParams(search: URLSearchParams): { slug: string; edition: string } | null {
  const slug = search.get('t') ?? '';
  const edition = search.get('e') ?? '';
  return SLUG.test(slug) && edition !== '' ? { slug, edition } : null;
}

/** editionHref is the address of an edition's page. */
export function editionHref(slug: string, edition: string): string {
  return `/blitz/?t=${encodeURIComponent(slug)}&e=${encodeURIComponent(edition)}`;
}

/** playersView is what players will see of the entrants: the active ones, without Discord names. */
export function playersView(entrants: readonly AdminBlitzEntrant[]): Pick<AdminBlitzEntrant, 'name' | 'zone' | 'utc_offset' | 'rating'>[] {
  return entrants.filter((e) => e.status === 'active').map(({ name, zone, utc_offset, rating }) => ({ name, zone, utc_offset, rating }));
}
