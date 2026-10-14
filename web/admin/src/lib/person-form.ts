/** The person form's decisions (adding a member, entering someone in an edition): whom the search offers, what picking one fills in, and what is sent. */
import type { components } from '@ttac/shared/api';
import { matchRank } from './match';

type Schemas = components['schemas'];
export type Candidate = Schemas['Candidate'];
export type NewBlitzEntrant = Schemas['NewBlitzEntrant'];
export type NewMember = Schemas['NewMember'];

/** PersonDetails is the form: the member picked (null for someone not yet a member), the name, and their details. */
export type PersonDetails = { member: number | null; name: string; discord_name: string; email: string; country: string; zone: string };
export const emptyPerson: PersonDetails = { member: null, name: '', discord_name: '', email: '', country: '', zone: 'Unknown' };

/** PersonChoice is someone the search offers, what it says of them, and the form picking them fills in. */
export type PersonChoice = { key: string; label: string; detail: string; disabled: boolean; fill: PersonDetails };

/** Taken says who the search may not offer: the names already entered in the edition, and whether every member is (when adding members). */
export type Taken = { entered: readonly string[]; membersTaken: boolean };

/** candidateChoice offers a candidate: a member with their details, or a player the ratings know. */
function candidateChoice(c: Candidate, membersTaken: boolean): PersonChoice {
  const member = c.member !== null;
  const what = !member ? 'in the ratings' : membersTaken ? 'member already' : 'member';
  const parts = [what, c.country ?? '', c.banned ? 'banned' : ''].filter((p) => p !== '');
  const fill = { ...emptyPerson, member: c.member, name: c.name, discord_name: c.discord_name, email: c.email, country: c.country ?? '', zone: c.zone ?? emptyPerson.zone };
  return { key: member ? `member ${c.member}` : `player ${c.name}`, label: c.name, detail: parts.join(' · '), disabled: c.banned || (member && membersTaken), fill };
}

/** personChoices lists what the search offers: everyone found, then, for text of two characters or more that nobody found has as their name, adding it as a new player, or saying an entrant has it. */
export function personChoices(found: readonly Candidate[], typed: string, taken: Taken): PersonChoice[] {
  const name = typed.trim();
  const out = found.map((c) => candidateChoice(c, taken.membersTaken));
  const same = (n: string) => n.toLowerCase() === name.toLowerCase();
  if (name.length < 2 || found.some((c) => same(c.name) || same(c.player))) return out;
  const entrant = taken.entered.find(same);
  if (entrant !== undefined) {
    out.push({ key: 'entered', label: entrant, detail: 'entered already', disabled: true, fill: emptyPerson });
  } else {
    out.push({ key: 'new', label: `Add “${name}” as a new player`, detail: 'not in the ratings', disabled: false, fill: { ...emptyPerson, name } });
  }
  return out;
}

/** details is the form's details as sent: trimmed, an empty one left out, the zone always. */
function details(f: PersonDetails): Pick<NewMember, 'discord_name' | 'email' | 'country' | 'zone'> {
  const out: Pick<NewMember, 'discord_name' | 'email' | 'country' | 'zone'> = { zone: f.zone };
  const discord = f.discord_name.trim();
  const email = f.email.trim();
  const country = f.country.trim();
  if (discord !== '') out.discord_name = discord;
  if (email !== '') out.email = email;
  if (country !== '') out.country = country;
  return out;
}

/** entrantToAdd is what the form sends to enter someone: the member by id, else the name, with every detail filled in. */
export function entrantToAdd(f: PersonDetails, pending: boolean): NewBlitzEntrant {
  const who = f.member !== null ? { member: f.member } : { name: f.name.trim() };
  return { ...who, ...details(f), status: pending ? 'pending' : 'active' };
}

/** memberToAdd is what the form sends to add a member: the name, with every detail filled in. */
export function memberToAdd(f: PersonDetails): NewMember {
  return { name: f.name.trim(), ...details(f) };
}

/** Outsider is an active entrant not in a knockout round: one a wildcard may be. */
export type Outsider = { id: number; name: string };

/** wildcardChoices lists whom a wildcard may be for the text typed: the entrants not in the round who match it, then the people found as personChoices offers them (entered: every entrant's name); nothing for no text. */
export function wildcardChoices(outsiders: readonly Outsider[], found: readonly Candidate[], typed: string, entered: readonly string[]): PersonChoice[] {
  const t = typed.trim();
  if (t === '') return [];
  const theirs = outsiders
    .filter((o) => matchRank(o.name, t) !== null)
    .map((o) => ({ key: `entrant ${o.id}`, label: o.name, detail: 'entrant', disabled: false, fill: { ...emptyPerson, name: o.name } }));
  const named = outsiders.some((o) => o.name.toLowerCase() === t.toLowerCase());
  const others = personChoices(found, t, { entered, membersTaken: false }).filter((c) => !(named && (c.key === 'entered' || c.key === 'new')));
  return [...theirs, ...others];
}
