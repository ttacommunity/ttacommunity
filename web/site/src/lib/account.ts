/** A player's account on the site: who is signed in, what the account page and nav show for them, and their requests to the api, made with the session cookie. */
import type { components } from '@ttac/shared/api';
import type { Question } from '@ttac/shared/Dialog';
import { errorFields } from '@ttac/shared/ratings';
import { playerHref } from './player-view';
import { ZONE_COUNTRIES } from './zone-countries';

export type Me = components['schemas']['Me'];
export type MyMember = components['schemas']['MyMember'];
type Provider = Me['provider'];

/** AccountState is what the account page shows: signed out; signed in but not linked, waiting for an answer, or refused; or linked to a member. */
export type AccountState =
  | { kind: 'signed-out' }
  | { kind: 'unlinked' }
  | { kind: 'waiting'; name: string }
  | { kind: 'refused'; name: string; reason: string }
  | { kind: 'linked'; member: MyMember; playerHref: string | null };

/** accountState reads who is signed in into what the account page shows. */
export function accountState(me: Me | null): AccountState {
  if (me === null) return { kind: 'signed-out' };
  if (me.member !== null) {
    return { kind: 'linked', member: me.member, playerHref: me.member.player === '' ? null : playerHref(me.member.player) };
  }
  if (me.request?.status === 'pending') return { kind: 'waiting', name: me.request.name };
  if (me.request?.status === 'refused') return { kind: 'refused', name: me.request.name, reason: me.request.reason };
  return { kind: 'unlinked' };
}

/** banNotice is a banned member's ban, with the reason the admin gave ('' for none), or null for anyone else. */
export function banNotice(state: AccountState): { reason: string } | null {
  return state.kind === 'linked' && state.member.banned ? { reason: state.member.ban_reason } : null;
}

/** deleteQuestion asks before deleting the account, which cannot be undone, saying what goes: every way in, and a linked member's Discord name, country, zone and an email from the sign-ins. */
export function deleteQuestion(state: AccountState): Question {
  const text =
    state.kind === 'linked'
      ? `Every way you sign in goes, with every session, and the Discord name, country and time zone on ${state.member.name}, and an email address your sign-ins gave it, are cleared. ${state.member.name}'s games and ratings stay. This cannot be undone.`
      : 'Every way you sign in goes, with every session, your requests to be linked and the details you gave. This cannot be undone.';
  return { heading: 'Delete your account?', text, action: 'Delete account', final: true };
}

/** UNNAMED is what the api calls a Microsoft account that has no name. */
const UNNAMED = 'Microsoft account';

/** navLabel is the nav's account link: Sign in, a member's TTA username, their Discord name, or their Microsoft account's name; never an email address, which the page would show to anyone looking. */
export function navLabel(me: Me | null): string {
  if (me === null) return 'Sign in';
  if (me.member !== null) return me.member.name;
  if (me.provider === 'discord') return me.display_name || me.username;
  const discord = me.logins.find((l) => l.provider === 'discord');
  if (discord !== undefined) return discord.display_name || discord.username;
  const microsoft = me.logins.find((l) => l.provider === 'microsoft' && l.username !== UNNAMED);
  return microsoft === undefined ? 'Your account' : microsoft.username;
}

/** showsAdminLink says whether the nav offers the way to admin: to admins and organisers, a convenience only, as admin itself is behind Access. */
export function showsAdminLink(me: Me | null): boolean {
  return me !== null && me.admin_role !== null;
}

/** signedInAs heads the account page: how this browser signed in and as whom, with the Discord username under a display name. */
export function signedInAs(me: Me): { heading: string; detail: string } {
  if (me.provider === 'email') return { heading: `Signed in by email as ${me.username}`, detail: '' };
  if (me.provider === 'microsoft') return { heading: `Signed in with Microsoft as ${me.username}`, detail: '' };
  return { heading: `Signed in with Discord as ${me.display_name || me.username}`, detail: me.display_name === '' ? '' : `Discord username: ${me.username}` };
}

/** PROVIDER_NAMES names each way in as the page shows it; a new provider in the api's types will not compile until it is named here. */
const PROVIDER_NAMES: Record<Provider, string> = { discord: 'Discord', email: 'Email', microsoft: 'Microsoft', google: 'Google' };

/** WayIn is one way into the account as the page lists it, whether it may be removed (only while another remains), and whether it may be changed (an email address, for a new one). */
export type WayIn = { provider: Provider; label: string; removable: boolean; changeable: boolean };

/** waysIn lists the account's ways in, and which kinds may be added. */
export function waysIn(me: Me): { ways: WayIn[]; canAdd: Record<Provider, boolean> } {
  const removable = me.logins.length > 1;
  const ways = me.logins.map((l) => {
    const also = l.provider === 'microsoft' || l.provider === 'google' ? l.email : l.display_name;
    const name = also === '' || also === l.username ? l.username : `${l.username} (${also})`;
    return { provider: l.provider, label: `${PROVIDER_NAMES[l.provider]}: ${name}`, removable, changeable: l.provider === 'email' };
  });
  const has = (p: Provider) => me.logins.some((l) => l.provider === p);
  return { ways, canAdd: { discord: !has('discord'), email: !has('email'), microsoft: !has('microsoft'), google: !has('google') } };
}

/** Signer is a way in that signs in through its own site: Discord or Microsoft. */
export type Signer = 'discord' | 'microsoft';

/** signInHref opens the api's sign-in with provider, which comes back to the page at back. */
export function signInHref(apiUrl: string, provider: Signer, back: string): string {
  return `${apiUrl.replace(/\/+$/, '')}/v1/auth/${provider}?${new URLSearchParams({ return: back })}`;
}

/** addWayInHref opens the api's sign-in with provider to add it to the signed-in account, which comes back to the account page. */
export function addWayInHref(apiUrl: string, provider: Signer): string {
  return `${apiUrl.replace(/\/+$/, '')}/v1/auth/${provider}?add=true`;
}

/** needsContact says whether the account gives organisers no way to reach the player: no Discord, and no email from any way in. */
export function needsContact(me: Me): boolean {
  return !me.logins.some((l) => l.provider === 'discord' || l.email !== '');
}

const ACCOUNT = '/account/';

/** backPath is a page on the site to go back to after signing in, with any query, or null for anything else: another site, a fragment, the account page itself. The api applies the same rules. */
export function backPath(value: string | null): string | null {
  if (value === null || value.length > 300 || !value.startsWith('/') || value.startsWith('//') || /[\\#]/.test(value)) return null;
  if (!/^[\x21-\x7e]+$/.test(value) || value === ACCOUNT || value.startsWith(ACCOUNT + '?')) return null;
  return value;
}

/** accountHref is the nav's link to the account page, carrying the page it is on as the way back. */
export function accountHref(path: string, search: string): string {
  const back = backPath(withoutSignInParam(path, search));
  return back === null ? ACCOUNT : `${ACCOUNT}?${new URLSearchParams({ back })}`;
}

/** isFirstSignIn says whether the api sent the browser here from an account's first sign-in. */
export function isFirstSignIn(params: URLSearchParams): boolean {
  return params.get('signin') === 'first';
}

/** backLabel names the way back from the account page: plain when nothing changed, a warning when changes would be lost, and an invitation once saved. */
export function backLabel(changed: boolean, saved: boolean): string {
  if (saved) return 'Back to where you were';
  return changed ? 'Back without saving' : 'Back';
}

/** SignInNotice is what the account page says on arriving: a sign-in or an addition that did not finish, a way in added, or an account just deleted. */
export type SignInNotice = { kind: 'failed' | 'deleted' | 'added'; text: string };

/** signInNotice reads the address the browser arrived at: a sign-in that did not finish, a way in added or refused (naming it), an account just deleted, or nothing to say. */
export function signInNotice(params: URLSearchParams): SignInNotice | null {
  const way = params.get('way') === 'microsoft' ? 'Microsoft' : 'Discord';
  switch (params.get('signin')) {
    case 'failed':
      return { kind: 'failed', text: 'Signing in did not finish. Try again.' };
    case 'deleted':
      return { kind: 'deleted', text: 'Your account is deleted, and this browser is signed out. Sign in again any time to start a new one.' };
    case 'added':
      return { kind: 'added', text: `${way} is added: you can sign in with it from now on.` };
    case 'email-added':
      return { kind: 'added', text: 'Your email address is saved: you can sign in with a link sent to it from now on.' };
    case 'taken':
      return {
        kind: 'failed',
        text: `That ${way} account has an account of its own here. To add it, sign in with it and delete that account, then sign in here again and add it.`,
      };
    case 'has-kind':
      return { kind: 'failed', text: `Your account has a ${way} account already. Remove it first to add another.` };
  }
  return null;
}

/** withoutSignInParam is the page's address without the sign-in's outcome, to put back in the address bar. */
export function withoutSignInParam(path: string, search: string): string {
  const params = new URLSearchParams(search);
  params.delete('signin');
  const rest = params.toString();
  return rest === '' ? path : `${path}?${rest}`;
}

const SIGNED_IN = 'ttac.signedIn';

/** browserStorage is the browser's local storage, or null where touching it throws (blocked site data). */
export function browserStorage(): Storage | null {
  try {
    return window.localStorage;
  } catch {
    return null;
  }
}

/** rememberSignedIn marks this browser as one that has signed in, or not, so pages ask the api who it is only then; a refusing store, or none, is ignored. */
export function rememberSignedIn(store: Storage | null, on: boolean): void {
  if (store === null) return;
  try {
    if (on) store.setItem(SIGNED_IN, '1');
    else store.removeItem(SIGNED_IN);
  } catch {
    // Private windows and blocked storage refuse; the nav then shows Sign in until the account page is opened.
  }
}

/** seemsSignedIn says whether this browser has signed in and not out since. */
export function seemsSignedIn(store: Storage | null): boolean {
  if (store === null) return false;
  try {
    return store.getItem(SIGNED_IN) === '1';
  } catch {
    return false;
  }
}

/** zoneOptions are the zones a player may choose: Unknown, the zone they hold when the browser does not know it (a region an organiser set), then every zone the browser knows, in order. */
export function zoneOptions(supported: readonly string[], current: string): string[] {
  const known = [...supported].sort();
  return ['Unknown', ...(current === 'Unknown' || known.includes(current) ? [] : [current]), ...known];
}

/** Country is a country a player may give: its code and its name in English. */
export type Country = { code: string; name: string };

/** countryOptions names each code in English (the code itself where there is no name), sorted by name. */
export function countryOptions(codes: readonly string[], names: Intl.DisplayNames | null): Country[] {
  return codes.map((code) => ({ code, name: names?.of(code) ?? code })).sort((a, b) => a.name.localeCompare(b.name));
}

/** suggestedCountry is the country to offer: the ratings player's whose name was typed (any case), else the country of the zone chosen, else of the browser's zone; null when none is known. */
export function suggestedCountry(players: readonly { name: string; country: string | null }[], typed: string, zone: string, browserZone: string): string | null {
  const name = typed.trim().toLowerCase();
  const player = name === '' ? undefined : players.find((p) => p.name.toLowerCase() === name);
  return player?.country ?? ZONE_COUNTRIES[zone] ?? ZONE_COUNTRIES[browserZone] ?? null;
}

/** AccountApiError is an answer from the api other than the one asked for. */
export class AccountApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
  ) {
    super(message);
  }
}

/** failureMessage reads an error answer's message, with its request id when the api gave one. */
export async function failureMessage(res: Response): Promise<string> {
  const { error, requestId } = errorFields(await res.json().catch(() => null));
  return (error ?? `request failed (${res.status})`) + (requestId === undefined ? '' : ` (ref ${requestId})`);
}

function url(base: string, path: string): string {
  return base.replace(/\/+$/, '') + path;
}

/** fetchMe returns who is signed in on this browser, or null for nobody. */
export async function fetchMe(base: string, fetchFn: typeof fetch = fetch): Promise<Me | null> {
  const res = await fetchFn(url(base, '/v1/me'), { credentials: 'include' });
  if (res.status === 401) return null;
  if (!res.ok) throw new AccountApiError(res.status, await failureMessage(res));
  return (await res.json()) as Me;
}

// One answer per page for the nav and the account page, as browsers do not all merge identical requests.
const shared = new WeakMap<typeof fetch, Map<string, Promise<Me | null>>>();

/** sharedMe is fetchMe asked once per page for every caller; a failure is asked again. */
export function sharedMe(base: string, fetchFn: typeof fetch = fetch): Promise<Me | null> {
  const byBase = shared.get(fetchFn) ?? new Map<string, Promise<Me | null>>();
  shared.set(fetchFn, byBase);
  let me = byBase.get(base);
  if (me === undefined) {
    me = fetchMe(base, fetchFn);
    byBase.set(base, me);
    me.catch(() => byBase.delete(base));
  }
  return me;
}

/** fetchCountries returns the countries a player may give, as codes. */
export async function fetchCountries(base: string, fetchFn: typeof fetch = fetch): Promise<string[]> {
  const res = await fetchFn(url(base, '/v1/countries'), { credentials: 'omit' });
  if (!res.ok) throw new AccountApiError(res.status, await failureMessage(res));
  return (await res.json()) as string[];
}

/** Done is how a change went: done, or refused with the api's message. */
export type Done = { ok: true } | { ok: false; message: string };

/** change sends a change to the api with the session cookie, answering how it went rather than throwing. */
export async function change(fetchFn: typeof fetch, base: string, method: string, path: string, body?: unknown): Promise<Done> {
  let res: Response;
  try {
    res = await fetchFn(url(base, path), {
      method,
      credentials: 'include',
      ...(body === undefined ? {} : { headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }),
    });
  } catch {
    return { ok: false, message: 'The site could not reach its server. Check your connection and try again.' };
  }
  return res.ok ? { ok: true } : { ok: false, message: await failureMessage(res) };
}

/** askToBeLinked asks for the signed-in player to be linked to the TTA username name. */
export function askToBeLinked(base: string, name: string, fetchFn: typeof fetch = fetch): Promise<Done> {
  return change(fetchFn, base, 'PUT', '/v1/me/link-request', { name: name.trim() });
}

/** changeMyDetails changes the player's own country ('' for none) and zone: their member's once linked, kept with the sign-in until then. */
export function changeMyDetails(base: string, details: { country: string; zone: string }, fetchFn: typeof fetch = fetch): Promise<Done> {
  return change(fetchFn, base, 'PATCH', '/v1/me/details', details);
}

/** Draft is the account form: the TTA username asked for, the country ('' for none) and the zone. */
export type Draft = { name: string; country: string; zone: string };

/** draftChanges says what saving the form would send: the request to be linked, the details, both or neither. */
export function draftChanges(saved: Draft, draft: Draft): { name: boolean; details: boolean } {
  const name = draft.name.trim();
  return { name: name !== '' && name !== saved.name.trim(), details: draft.country !== saved.country || draft.zone !== saved.zone };
}

/** SaveOutcome is what a save of the account form did, in the words admin uses: Done, or Not done and why. */
export type SaveOutcome = { kind: 'done' | 'refused'; heading: 'Done' | 'Not done'; text: string };

/** saveOutcome says what a save did from the request to be linked and the details change, each null when not sent. */
export function saveOutcome(asked: Done | null, details: Done | null): SaveOutcome {
  const refused = (text: string): SaveOutcome => ({ kind: 'refused', heading: 'Not done', text });
  const done = (text: string): SaveOutcome => ({ kind: 'done', heading: 'Done', text });
  if (asked !== null && !asked.ok) return refused(asked.message);
  if (details !== null && !details.ok) {
    return refused(asked === null ? details.message : `Sent your request to be linked, but your country and time zone were not saved: ${details.message}`);
  }
  if (asked !== null && details !== null) return done('Sent your request to be linked, and saved your country and time zone.');
  if (asked !== null) return done('Sent your request to be linked.');
  if (details !== null) return done('Saved your country and time zone.');
  return refused('Nothing to save.');
}

/** addEmail emails a link that adds the address to the signed-in player's account, in place of any it has. */
export function addEmail(base: string, email: string, fetchFn: typeof fetch = fetch): Promise<Done> {
  return change(fetchFn, base, 'POST', '/v1/me/logins/email', { email: email.trim() });
}

/** removeWayIn removes one of the signed-in player's ways in. */
export function removeWayIn(base: string, provider: Provider, fetchFn: typeof fetch = fetch): Promise<Done> {
  return change(fetchFn, base, 'DELETE', `/v1/me/logins/${provider}`);
}

/** deleteAccount deletes the signed-in player's account and signs this browser out. */
export function deleteAccount(base: string, fetchFn: typeof fetch = fetch): Promise<Done> {
  return change(fetchFn, base, 'DELETE', '/v1/me');
}

/** signOut ends this browser's session. */
export function signOut(base: string, fetchFn: typeof fetch = fetch): Promise<Done> {
  return change(fetchFn, base, 'POST', '/v1/auth/sign-out');
}
