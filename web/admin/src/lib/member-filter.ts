/** The Members tab's list: which members it shows for the statuses ticked and the text typed. */
import type { AdminMember } from './admin-api';
import { matchRank } from './match';

/** MemberStatus is how a member comes to be here: a sign-in is linked to them, they are entered in an edition, or they are from our data alone (a player given a country). */
export type MemberStatus = 'account' | 'entrant' | 'data';

/** MEMBER_STATUSES are the statuses as the Show fieldset lists them. */
export const MEMBER_STATUSES: readonly { status: MemberStatus; label: string }[] = [
  { status: 'account', label: 'Has an account' },
  { status: 'entrant', label: 'Entrant' },
  { status: 'data', label: 'Data only' },
];

/** DEFAULT_STATUSES are the statuses shown at first: everyone but the members from the data alone. */
export const DEFAULT_STATUSES: ReadonlySet<MemberStatus> = new Set<MemberStatus>(['account', 'entrant']);

/** memberStatuses returns a member's statuses: account when a sign-in is linked to them, entrant when they are in an edition, and data when neither. */
export function memberStatuses(m: AdminMember, linked: boolean): MemberStatus[] {
  const statuses: MemberStatus[] = [];
  if (linked) statuses.push('account');
  if (m.entries > 0) statuses.push('entrant');
  return statuses.length > 0 ? statuses : ['data'];
}

/** membersWithStatus keeps, in their order, the members any of whose statuses is ticked; linked holds the ids of the members a sign-in is linked to. */
export function membersWithStatus(members: readonly AdminMember[], ticked: ReadonlySet<MemberStatus>, linked: ReadonlySet<number>): AdminMember[] {
  return members.filter((m) => memberStatuses(m, linked.has(m.id)).some((s) => ticked.has(s)));
}

/** membersMatching keeps, in their order, the members matching any of the comma-separated names typed: by username, ratings spelling, Discord name or email as matchRank matches, or by their whole country code; everyone for no text. */
export function membersMatching(members: readonly AdminMember[], typed: string): AdminMember[] {
  const terms = typed
    .split(',')
    .map((t) => t.trim())
    .filter((t) => t !== '');
  if (terms.length === 0) return [...members];
  const matches = (m: AdminMember, t: string) =>
    (m.country ?? '').toLowerCase() === t.toLowerCase() || [m.name, m.player, m.discord_name, m.email].some((f) => f !== '' && matchRank(f, t) !== null);
  return members.filter((m) => terms.some((t) => matches(m, t)));
}
