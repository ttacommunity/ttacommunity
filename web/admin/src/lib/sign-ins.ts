/** The decisions behind players' sign-ins on the Members tab: which wait for an answer, which member each linked one belongs to, and what an approver is told. */
import type { AdminLogin } from './admin-api';

type Request = NonNullable<AdminLogin['request']>;

/** Hint is what an approver is told: approving adds a member, or how the member's recorded Discord name compares with the sign-in's. */
export type Hint = { kind: 'new' | 'unknown' | 'same' | 'differs'; text: string };

/** discordKey is a Discord name as the automatic link compares it: no case, no leading @. */
function discordKey(name: string): string {
  return name.trim().replace(/^@/, '').toLowerCase();
}

/** requestHint tells an approver what they are approving for the sign-in with that Discord username. */
export function requestHint(r: Request, username: string): Hint {
  if (r.member === '') return { kind: 'new', text: `No member is called ${r.name}: approving adds one.` };
  if (r.member_discord === '') return { kind: 'unknown', text: `${r.member} has no Discord name recorded: check it is them another way.` };
  if (discordKey(r.member_discord) === discordKey(username)) return { kind: 'same', text: `${r.member}'s recorded Discord name is ${r.member_discord}: the same.` };
  return { kind: 'differs', text: `${r.member}'s recorded Discord name is ${r.member_discord}, not ${username}.` };
}

/** DetailsHint is what an approver is told about the country and zone the player gave: none, given with nothing recorded to compare, the same as recorded, or different. */
export type DetailsHint = { kind: 'none' | 'given' | 'same' | 'differs'; text: string };

/** shown writes a country and zone as "NZ · Pacific/Auckland", naming what is missing. */
function shown(country: string, zone: string): string {
  return `${country || 'no country'} · ${zone || 'no time zone'}`;
}

/** detailsHint compares the country and zone a player gave with those recorded for the member their request names. */
export function detailsHint(r: Request): DetailsHint {
  if (r.country === '' && r.zone === '') return { kind: 'none', text: 'Gave no country or time zone.' };
  const gave = shown(r.country, r.zone);
  const recorded = r.member_country !== '' || (r.member_zone !== '' && r.member_zone !== 'Unknown');
  if (r.member === '' || !recorded) return { kind: 'given', text: `Gave ${gave}.` };
  const has = shown(r.member_country, r.member_zone);
  if (gave === has) return { kind: 'same', text: `Gave ${gave}: the same as recorded.` };
  return { kind: 'differs', text: `Gave ${gave}; recorded ${has}.` };
}

/** signInGroups splits the sign-ins three ways: waiting for an answer, linked (by their member's id) and the rest, in the api's order. */
export function signInGroups(rows: readonly AdminLogin[]): { waiting: AdminLogin[]; byMember: Map<number, AdminLogin>; unlinked: AdminLogin[] } {
  const byMember = new Map<number, AdminLogin>();
  for (const l of rows) if (l.member_id !== null) byMember.set(l.member_id, l);
  return { waiting: rows.filter((l) => l.request !== null), byMember, unlinked: rows.filter((l) => l.request === null && l.member_id === null) };
}

/** standing says where a sign-in neither linked nor waiting stands: refused (with the reason the player was shown), unlinked by an admin, or never asked. */
export function standing(l: AdminLogin): { text: string; detail: string } {
  if (l.refused !== null) return { text: `asked for ${l.refused.name}, refused`, detail: l.refused.reason };
  if (!l.auto_link) return { text: 'unlinked by an admin', detail: 'not linked by Discord name again' };
  return { text: 'has not asked to be linked', detail: '' };
}

/** howLinked says how a sign-in came to be linked: by its recorded Discord name, or by an approved request; empty when it is not. */
export function howLinked(l: AdminLogin): string {
  if (l.linked_how === 'match') return 'linked by Discord name';
  return l.linked_how === 'approved' ? 'approved' : '';
}
