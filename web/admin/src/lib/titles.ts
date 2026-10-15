/** The titles admin can set by hand, named as the site names them. */
import type { components } from '@ttac/shared/api';
import { titleName } from '@ttac/shared/ratings';

/** A title admin can set; '' takes a player's title away. */
export type AdminTitle = components['schemas']['TitleOverride']['title'];

/** ADMIN_TITLES are the titles admin offers, highest first; WC comes from the World Championship's podium. */
export const ADMIN_TITLES: readonly AdminTitle[] = ['SGM', 'WCS', 'WCB', 'GM', 'M', 'P', 'G', 'S', 'B', 'W', ''];

/** titleLabel returns how admin shows a title: the site's name and its code, or "No title". */
export function titleLabel(t: AdminTitle): string {
  return t === '' ? 'No title' : `${titleName(t)} (${t})`;
}
