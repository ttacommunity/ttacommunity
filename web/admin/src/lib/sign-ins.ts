/** The decisions behind players' accounts on the Members tab: which wait for an answer, which member each linked one belongs to, how each is named, and what an approver is told. */
import type { AdminAccount } from './admin-api';

type Request = NonNullable<AdminAccount['request']>;

/** who names an account by its ways in: each Discord username with its display name when it differs, the email address, and a Microsoft account's name, marked as Microsoft's (its email has its own column). */
export function who(a: AdminAccount): string {
  return a.logins
    .map((l) => {
      if (l.provider === 'microsoft') return l.username === 'Microsoft account' ? l.username : `${l.username} (Microsoft)`;
      return l.display_name === '' || l.display_name === l.username ? l.username : `${l.username} (${l.display_name})`;
    })
    .join(' · ');
}

/** Hint is what an approver is told: approving adds a member, or how the member's recorded Discord name and email compare with the account's. */
export type Hint = { kind: 'new' | 'unknown' | 'same' | 'differs'; text: string };

/** discordKey is a Discord name as the automatic link compares it: no case, no leading @. */
function discordKey(name: string): string {
  return name.trim().replace(/^@/, '').toLowerCase();
}

/** emailKey is an email address as the automatic link compares it: no case, no spaces around it. */
function emailKey(address: string): string {
  return address.trim().toLowerCase();
}

/** requestHint tells an approver what they are approving for the account: each recorded name it can be compared with, the same or not. */
export function requestHint(r: Request, a: AdminAccount): Hint {
  if (r.member === '') return { kind: 'new', text: `No member is called ${r.name}: approving adds one.` };
  const compared: { same: boolean; text: string }[] = [];
  const discord = a.logins.find((l) => l.provider === 'discord')?.username;
  if (discord !== undefined && r.member_discord !== '') {
    const same = discordKey(r.member_discord) === discordKey(discord);
    compared.push({ same, text: `recorded Discord name is ${r.member_discord}${same ? ': the same' : `, not ${discord}`}` });
  }
  const email = a.logins.find((l) => l.provider === 'email')?.username ?? a.logins.find((l) => l.email !== '')?.email;
  if (email !== undefined && r.member_email !== '') {
    const same = emailKey(r.member_email) === emailKey(email);
    compared.push({ same, text: `recorded email is ${r.member_email}${same ? ': the same' : `, not ${email}`}` });
  }
  if (compared.length === 0) return { kind: 'unknown', text: `${r.member} has no Discord name or email recorded to compare: check it is them another way.` };
  return { kind: compared.some((c) => c.same) ? 'same' : 'differs', text: `${r.member}'s ${compared.map((c) => c.text).join('; ')}.` };
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

/** signInGroups splits the accounts three ways: waiting for an answer, linked (by their member's id) and the rest, in the api's order. */
export function signInGroups(rows: readonly AdminAccount[]): { waiting: AdminAccount[]; byMember: Map<number, AdminAccount>; unlinked: AdminAccount[] } {
  const byMember = new Map<number, AdminAccount>();
  for (const l of rows) if (l.member_id !== null) byMember.set(l.member_id, l);
  return { waiting: rows.filter((l) => l.request !== null), byMember, unlinked: rows.filter((l) => l.request === null && l.member_id === null) };
}

/** standing says where an account neither linked nor waiting stands: refused (with the reason the player was shown), unlinked by an admin, or never asked. */
export function standing(l: AdminAccount): { text: string; detail: string } {
  if (l.refused !== null) return { text: `asked for ${l.refused.name}, refused`, detail: l.refused.reason };
  if (!l.auto_link) return { text: 'unlinked by an admin', detail: 'not linked by Discord name or email again' };
  return { text: 'has not asked to be linked', detail: '' };
}

/** howLinked says how an account came to be linked: by a recorded Discord name or email, or by an approved request; empty when it is not. */
export function howLinked(l: AdminAccount): string {
  if (l.linked_how === 'match') return 'linked by Discord name or email';
  return l.linked_how === 'approved' ? 'approved' : '';
}
