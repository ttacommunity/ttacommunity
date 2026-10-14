/** The blitz edition page's decisions: the edition the address names, its address, and advice on its format. */
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

/** formatAdvice is the most Swiss rounds the active entrants can always be paired for without a repeat (half of them,
 * rounded down, at least one), and warnings for a format past it or a knockout larger than the field. A warning only:
 * late entrants and wildcards raise the limit. */
export function formatAdvice(format: { swiss_rounds: number; knockout_size: number }, active: number): { limit: number; warnings: string[] } {
  const limit = Math.max(1, Math.floor(active / 2));
  const warnings: string[] = [];
  if (format.swiss_rounds > limit) {
    warnings.push(
      `${format.swiss_rounds} Swiss rounds is more than ${active} active entrants can always be paired for: past ${limit}, someone may have to meet an opponent again, and Pair refuses that. Late entrants or wildcards raise the limit.`,
    );
  }
  if (format.knockout_size > active) warnings.push(`The knockout takes the top ${format.knockout_size}, and ${active} entrants are active.`);
  return { limit, warnings };
}
