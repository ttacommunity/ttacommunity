/** The members list's decisions: who can still be entered in an edition, and what a country code looks like. */
import type { components } from '@ttac/shared/api';

type Schemas = components['schemas'];
export type AdminMember = Schemas['AdminMember'];
export type NewMember = Schemas['NewMember'];
export type MemberChange = Schemas['MemberChange'];

/** membersToEnter lists the members an edition can still take: not in it, not banned, by name without case. */
export function membersToEnter(members: readonly AdminMember[], entrants: readonly { member: number }[]): AdminMember[] {
  const entered = new Set(entrants.map((e) => e.member));
  return members.filter((m) => !m.banned && !entered.has(m.id)).sort((a, b) => a.name.localeCompare(b.name, undefined, { sensitivity: 'base' }));
}

/** isCountryCode says whether text is empty or two letters, the shape of a country code. */
export function isCountryCode(text: string): boolean {
  return /^([A-Za-z]{2})?$/.test(text.trim());
}
