/** The ways into a player's account, as the site and admin name them. */
import type { components } from '../api/schema';

/** Provider is a way in: Discord, an emailed link, Microsoft or Google. */
export type Provider = components['schemas']['Me']['provider'];

/** PROVIDER_NAMES names each way in as the pages show it; a new provider in the api's types will not compile until it is named here. */
export const PROVIDER_NAMES: Record<Provider, string> = { discord: 'Discord', email: 'Email', microsoft: 'Microsoft', google: 'Google' };

/** isOpenID says whether a way in is Microsoft or Google, whose login carries the account's own name and an email. */
export function isOpenID(p: Provider): p is 'microsoft' | 'google' {
  return p === 'microsoft' || p === 'google';
}

/** hasOwnName says whether a login's name is its own, not the "Microsoft account" or "Google account" the api gives one with none. */
export function hasOwnName(p: Provider, username: string): boolean {
  return !isOpenID(p) || username !== `${PROVIDER_NAMES[p]} account`;
}
