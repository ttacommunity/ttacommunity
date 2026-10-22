/** "Data we keep" on the account page: everything the api keeps about the signed-in player, grouped, each group saying when what it lists is removed. */
import type { components } from '@ttac/shared/api';
import { formatTime } from '@ttac/shared/time';
import { AccountApiError, failureMessage } from './account';

export type MyData = components['schemas']['MyData'];
type Provider = MyData['logins'][number]['provider'];

/** Fact is one detail kept: a key for finding it, its label, its value as shown, and a note saying what it is ('' for none). */
export type Fact = { key: string; label: string; value: string; note: string };

/** KeptList is a list of facts with an id, a title ('' for none), the rule saying when they are removed ('' when the group's says it), and whether it is this browser's session. */
export type KeptList = { id: string; title: string; rule: string; facts: Fact[]; thisBrowser: boolean };

/** KeptGroup is one group of what is kept: its id, heading, rule ('' when its lists carry their own), lists, and what to say when it has none. */
export type KeptGroup = { id: string; heading: string; rule: string; lists: KeptList[]; empty: string };

/** PROVIDERS names each way in and says what its id is; a new provider in the api's types will not compile until it is here. */
const PROVIDERS: Record<Provider, { name: string; id: string }> = {
  discord: { name: 'Discord', id: 'The number Discord knows you by' },
  email: { name: 'Email', id: 'Your email address' },
  microsoft: { name: 'Microsoft', id: 'The id Microsoft gives you for this site alone' },
  google: { name: 'Google', id: "Your Google account's id" },
};

const fact = (key: string, label: string, value: string, note = ''): Fact => ({ key, label, value, note });
const list = (id: string, title: string, rule: string, facts: Fact[], thisBrowser = false): KeptList => ({ id, title, rule, facts, thisBrowser });

/** keptGroups arranges what is kept into the dialog's groups, with times in zone and countries named by countryName. */
export function keptGroups(data: MyData, zone: string, countryName: (code: string) => string): KeptGroup[] {
  const when = (instant: string) => formatTime(instant, zone);
  // The player record first: it is the part a player knows best.
  const groups: KeptGroup[] = data.member === null ? [] : [memberGroup(data.member, when, countryName)];
  groups.push(
    { id: 'account', heading: 'Your account', rule: 'Removed if you delete your account.', lists: [list('account', '', '', accountFacts(data.account, when, countryName))], empty: '' },
    {
      id: 'logins',
      heading: 'Ways you sign in',
      rule: 'Each removed when you remove it, or if you delete your account.',
      lists: data.logins.map((l) => list(l.provider, PROVIDERS[l.provider].name, '', loginFacts(l, when))),
      empty: '',
    },
    {
      id: 'requests',
      heading: 'Requests to be linked',
      rule: 'Removed if you delete your account.',
      lists: data.requests.map((r, i) => list(`request-${i}`, r.name, '', requestFacts(r, when))),
      empty: 'None.',
    },
    {
      id: 'sessions',
      heading: 'Sessions',
      rule: 'Each removed when you sign out there, or 30 days after it was last used.',
      lists: data.sessions.map((s, i) =>
        list(`session-${i}`, PROVIDERS[s.provider].name + (s.this_browser ? ', this browser' : ''), '', [
          fact('started', 'Started', when(s.created_at)),
          fact('removed', 'Removed', when(s.ends_at), 'Later each time it is used'),
        ], s.this_browser),
      ),
      empty: '',
    },
    {
      id: 'links',
      heading: 'Sign-in links we emailed',
      rule: 'Each removed 48 hours after it was sent, so we can limit how many we send.',
      lists: data.email_links.map((l, i) =>
        list(`link-${i}`, l.address, '', [
          fact('sent', 'Sent', when(l.created_at)),
          fact('return_path', 'Returns to', l.return_path),
          fact('works_until', 'Works until', when(l.expires_at)),
          fact('used', 'Used', l.used_at === null ? 'Not used' : when(l.used_at)),
          fact('removed', 'Removed', when(l.kept_until)),
        ]),
      ),
      empty: 'None in the last 48 hours.',
    },
  );
  return groups;
}

function accountFacts(a: MyData['account'], when: (instant: string) => string, countryName: (code: string) => string): Fact[] {
  const how = a.linked_how === 'approved' ? 'By an approved request' : 'By your matching Discord name or email';
  const facts = [
    fact('created', 'Made', when(a.created_at)),
    fact('linked', 'Linked to your player record', a.linked_how === null || a.linked_at === null ? 'Not yet' : `${how}, ${when(a.linked_at)}`),
    fact('auto_link', 'Linking by a matching Discord name or email', a.auto_link ? 'On' : 'Off: an admin unlinked this account'),
  ];
  // Once linked, the country and zone given before moved to the player record.
  if (a.linked_how === null || a.country !== null) facts.push(fact('country', 'Country you gave', a.country === null ? 'None given' : countryName(a.country)));
  if (a.linked_how === null || a.zone !== null) facts.push(fact('zone', 'Time zone you gave', a.zone ?? 'None given'));
  return facts;
}

function loginFacts(l: MyData['logins'][number], when: (instant: string) => string): Fact[] {
  const facts = [fact('id', 'Id', l.id, PROVIDERS[l.provider].id)];
  if (l.username !== l.id) facts.push(fact('name', 'Name', l.username));
  if (l.display_name !== '') facts.push(fact('display_name', 'Display name', l.display_name));
  if (l.email !== '') facts.push(fact('email', 'Email it gave us', l.email));
  facts.push(fact('added', 'Added', when(l.created_at)), fact('last_used', 'Last used', when(l.signed_in_at)));
  return facts;
}

function memberGroup(m: NonNullable<MyData['member']>, when: (instant: string) => string, countryName: (code: string) => string): KeptGroup {
  const email = m.email === '' ? [] : [fact('email', 'Email', m.email)];
  const stays = [
    fact('tta_name', 'TTA username', m.name),
    fact('cge_user', 'CGE user number', m.cge_user === null ? 'Not known' : String(m.cge_user)),
    fact('player', 'Name on the ratings pages', m.player === '' ? 'None yet: no rated games' : m.player),
    ...(m.email_cleared ? [] : email),
    fact('ban', 'Ban', !m.banned ? 'None' : `Since ${m.banned_at === null ? 'an unknown date' : when(m.banned_at)}${m.ban_reason === '' ? '' : `: ${m.ban_reason}`}`),
    fact('created', 'Record made', when(m.created_at)),
  ];
  const cleared = [
    ...(m.discord_name === '' ? [] : [fact('discord_name', 'Discord name', m.discord_name)]),
    ...(m.email_cleared ? email : []),
    ...(m.country === null ? [] : [fact('country', 'Country', countryName(m.country))]),
    fact('zone', 'Time zone', m.zone),
  ];
  return {
    id: 'member',
    heading: 'Your player record',
    rule: 'The tournaments’ record of you. To change or remove what stays, write to privacy@ttacommunity.org.',
    lists: [list('stays', 'Stays if you delete your account', '', stays), list('cleared', 'Cleared if you delete your account', '', cleared)],
    empty: '',
  };
}

function requestFacts(r: MyData['requests'][number], when: (instant: string) => string): Fact[] {
  const answered = r.decided_at === null ? '' : ` ${when(r.decided_at)}`;
  const answer = r.status === 'pending' ? 'Waiting for an answer' : `${r.status === 'approved' ? 'Approved' : 'Refused'}${answered}`;
  const facts = [fact('asked', 'Asked', when(r.requested_at)), fact('answer', 'Answer', answer)];
  if (r.status === 'refused' && r.reason !== '') facts.push(fact('reason', 'Reason', r.reason));
  return facts;
}

/** shownZone is the zone the dialog's times are in: the player's saved zone when it is an IANA name, else the browser's. */
export function shownZone(saved: string, browser: string): string {
  try {
    new Intl.DateTimeFormat('en-GB', { timeZone: saved });
    return saved;
  } catch {
    return browser;
  }
}

/** fetchMyData returns everything kept about the signed-in player, or null when nobody is signed in. */
export async function fetchMyData(base: string, fetchFn: typeof fetch = fetch): Promise<MyData | null> {
  const res = await fetchFn(`${base.replace(/\/+$/, '')}/v1/me/data`, { credentials: 'include' });
  if (res.status === 401) return null;
  if (!res.ok) throw new AccountApiError(res.status, await failureMessage(res));
  return (await res.json()) as MyData;
}
