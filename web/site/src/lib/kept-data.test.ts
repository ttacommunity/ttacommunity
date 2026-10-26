import { describe, expect, it } from 'vitest';
import { formatTime } from '@ttac/shared/time';
import { fetchMyData, keptGroups, shownZone, type KeptGroup, type MyData } from './kept-data';

const ZONE = 'Australia/Sydney';
const at = (instant: string) => formatTime(instant, ZONE);
const country = (code: string) => `country ${code}`;

const member: NonNullable<MyData['member']> = {
  name: 'Ann',
  cge_user: 501,
  player: 'Ann',
  discord_name: 'player_one',
  email: 'ann@example.test',
  email_cleared: false,
  country: 'NZ',
  zone: 'Pacific/Auckland',
  banned: false,
  ban_reason: '',
  banned_at: null,
  created_at: '2026-10-01T00:00:00Z',
};

const data: MyData = {
  account: { created_at: '2026-10-18T12:00:00Z', country: null, zone: null, linked_how: 'approved', linked_at: '2026-10-18T13:00:00Z', auto_link: true },
  logins: [
    { provider: 'discord', id: '100000000000000001', username: 'player_one', display_name: 'Player One', email: '', created_at: '2026-10-18T12:00:00Z', signed_in_at: '2026-10-20T12:00:00Z' },
    { provider: 'email', id: 'ann@example.test', username: 'ann@example.test', display_name: '', email: '', created_at: '2026-10-19T12:00:00Z', signed_in_at: '2026-10-19T12:00:00Z' },
    { provider: 'google', id: '100000000000000000002', username: 'Ann', display_name: '', email: 'ann@gmail.example', created_at: '2026-10-19T12:00:00Z', signed_in_at: '2026-10-19T12:00:00Z' },
    { provider: 'microsoft', id: 'AAAAAAAAAAAAAAAAAAAAAIkzqFVrSaSaFHy782bbtaQ', username: 'Ann', display_name: '', email: '', created_at: '2026-10-19T12:00:00Z', signed_in_at: '2026-10-19T12:00:00Z' },
  ],
  member,
  requests: [{ name: 'Ann', status: 'approved', reason: '', requested_at: '2026-10-18T12:30:00Z', decided_at: '2026-10-18T13:00:00Z' }],
  sessions: [
    { provider: 'discord', created_at: '2026-10-18T12:00:00Z', ends_at: '2026-11-17T12:00:00Z', this_browser: false },
    { provider: 'email', created_at: '2026-10-20T12:00:00Z', ends_at: '2026-11-19T12:00:00Z', this_browser: true },
  ],
  email_links: [
    { address: 'ann@example.test', return_path: '/account/', created_at: '2026-10-21T12:00:00Z', expires_at: '2026-10-21T13:00:00Z', used_at: null, kept_until: '2026-10-23T12:00:00Z' },
  ],
};

/** group finds a group by id, failing the test when it is missing. */
function group(groups: KeptGroup[], id: string): KeptGroup {
  const g = groups.find((x) => x.id === id);
  if (g === undefined) throw new Error(`no group ${id} in ${groups.map((x) => x.id).join(', ')}`);
  return g;
}

/** fact is the value of the fact key in the list listId of group groupId, or undefined when it is not there. */
function fact(groups: KeptGroup[], groupId: string, listId: string, key: string): string | undefined {
  return group(groups, groupId)
    .lists.find((l) => l.id === listId)
    ?.facts.find((f) => f.key === key)?.value;
}

describe('keptGroups', () => {
  it('lists the player record and its challenge dominations first, as the part a player knows best, and leaves them out before a link', () => {
    expect(keptGroups(data, ZONE, country).map((g) => g.id)).toEqual(['member', 'challenges', 'account', 'logins', 'requests', 'sessions', 'links']);
    expect(keptGroups({ ...data, member: null }, ZONE, country).map((g) => g.id)).toEqual(['account', 'logins', 'requests', 'sessions', 'links']);
  });

  it('gives every group a rule saying when what it lists is removed', () => {
    const groups = keptGroups(data, ZONE, country);
    expect(groups).toHaveLength(7);
    for (const g of groups) {
      const rules = [g.rule, ...g.lists.map((l) => l.rule)].filter((r) => r !== '');
      expect(rules.length, g.id).toBeGreaterThan(0);
    }
  });

  it.each([
    ['discord', '100000000000000001'],
    ['email', 'ann@example.test'],
    ['google', '100000000000000000002'],
    ['microsoft', 'AAAAAAAAAAAAAAAAAAAAAIkzqFVrSaSaFHy782bbtaQ'],
  ])('shows the %s way in by its id, saying what the id is', (provider, id) => {
    const way = group(keptGroups(data, ZONE, country), 'logins').lists.find((l) => l.id === provider);
    const idFact = way?.facts.find((f) => f.key === 'id');
    expect(idFact?.value).toBe(id);
    expect(idFact?.note).not.toBe('');
  });

  it('gives each way in its names, the email it gave, and when it was added and last used', () => {
    const groups = keptGroups(data, ZONE, country);
    expect(fact(groups, 'logins', 'discord', 'display_name')).toBe('Player One');
    expect(fact(groups, 'logins', 'google', 'email')).toBe('ann@gmail.example');
    expect(fact(groups, 'logins', 'discord', 'added')).toBe(at('2026-10-18T12:00:00Z'));
    expect(fact(groups, 'logins', 'discord', 'last_used')).toBe(at('2026-10-20T12:00:00Z'));
  });

  it("leaves out an email way in's name, which is its id again, and names and emails a way in does not have", () => {
    const groups = keptGroups(data, ZONE, country);
    expect(fact(groups, 'logins', 'email', 'name')).toBeUndefined();
    expect(fact(groups, 'logins', 'microsoft', 'email')).toBeUndefined();
    expect(fact(groups, 'logins', 'google', 'display_name')).toBeUndefined();
  });

  it.each([
    ['an email an admin recorded', false, 'stays'],
    ["an email a way in gave", true, 'cleared'],
  ])('splits the player record by what deleting the account does: %s', (_name, cleared, list) => {
    const groups = keptGroups({ ...data, member: { ...member, email_cleared: cleared } }, ZONE, country);
    expect(fact(groups, 'member', list, 'email')).toBe('ann@example.test');
    expect(fact(groups, 'member', list === 'stays' ? 'cleared' : 'stays', 'email')).toBeUndefined();
  });

  it.each([
    ['tta_name', 'stays'],
    ['cge_user', 'stays'],
    ['player', 'stays'],
    ['ban', 'stays'],
    ['created', 'stays'],
    ['discord_name', 'cleared'],
    ['country', 'cleared'],
    ['zone', 'cleared'],
  ])('keeps the player record’s %s in the list for what deleting the account does to it (%s)', (key, list) => {
    expect(fact(keptGroups(data, ZONE, country), 'member', list, key)).not.toBeUndefined();
  });

  it.each([
    ['not banned', member, 'None'],
    ['banned', { ...member, banned: true, ban_reason: 'Abusive', banned_at: '2026-10-19T00:00:00Z' }, `Since ${at('2026-10-19T00:00:00Z')}: Abusive`],
  ])('shows a ban when %s', (_name, m, want) => {
    expect(fact(keptGroups({ ...data, member: m }, ZONE, country), 'member', 'stays', 'ban')).toBe(want);
  });

  it('names the country, and leaves out a Discord name, email or country the record does not have', () => {
    expect(fact(keptGroups(data, ZONE, country), 'member', 'cleared', 'country')).toBe('country NZ');
    const bare = keptGroups({ ...data, member: { ...member, discord_name: '', email: '', country: null } }, ZONE, country);
    for (const [list, key] of [['cleared', 'discord_name'], ['stays', 'email'], ['cleared', 'country']]) {
      expect(fact(bare, 'member', list, key), key).toBeUndefined();
    }
  });

  it.each([
    ['linked by a request', { linked_how: 'approved' as const }, true],
    ['linked by a matching name', { linked_how: 'match' as const }, true],
    ['not linked', { linked_how: null, linked_at: null }, false],
  ])('says how the account was linked: %s', (_name, change, linked) => {
    const value = fact(keptGroups({ ...data, account: { ...data.account, ...change } }, ZONE, country), 'account', 'account', 'linked');
    expect(value?.includes(at('2026-10-18T13:00:00Z'))).toBe(linked);
  });

  it.each([
    ['before a link, the ones given', { linked_how: null, linked_at: null, country: 'NZ', zone: 'Pacific/Auckland' }, 'country NZ', 'Pacific/Auckland'],
    ['before a link, none given', { linked_how: null, linked_at: null }, 'None given', 'None given'],
    ['once linked, none: they moved to the record', {}, undefined, undefined],
  ])("shows the account's own country and zone %s", (_name, change, wantCountry, wantZone) => {
    const groups = keptGroups({ ...data, account: { ...data.account, ...change } }, ZONE, country);
    expect(fact(groups, 'account', 'account', 'country')).toBe(wantCountry);
    expect(fact(groups, 'account', 'account', 'zone')).toBe(wantZone);
  });

  it.each([
    ['pending', { status: 'pending' as const, decided_at: null }, 'Waiting for an answer'],
    ['approved', {}, `Approved ${at('2026-10-18T13:00:00Z')}`],
    ['refused', { status: 'refused' as const, reason: 'Not you' }, `Refused ${at('2026-10-18T13:00:00Z')}`],
  ])('gives a request its answer when %s', (_name, change, want) => {
    const groups = keptGroups({ ...data, requests: [{ ...data.requests[0], ...change }] }, ZONE, country);
    expect(group(groups, 'requests').lists[0].facts.find((f) => f.key === 'answer')?.value).toBe(want);
  });

  it("gives a refused request's reason", () => {
    const groups = keptGroups({ ...data, requests: [{ ...data.requests[0], status: 'refused', reason: 'Not you' }] }, ZONE, country);
    expect(group(groups, 'requests').lists[0].facts.find((f) => f.key === 'reason')?.value).toBe('Not you');
  });

  it("gives each session its way in, when it started and when it is removed, marking this browser's", () => {
    const sessions = group(keptGroups(data, ZONE, country), 'sessions').lists;
    expect(sessions.map((s) => s.thisBrowser)).toEqual([false, true]);
    expect(sessions[1].facts.find((f) => f.key === 'removed')?.value).toBe(at('2026-11-19T12:00:00Z'));
    expect(sessions[1].facts.find((f) => f.key === 'started')?.value).toBe(at('2026-10-20T12:00:00Z'));
  });

  it('gives each emailed link where it went, where it returns, when it was sent and used, and when it is removed', () => {
    const link = group(keptGroups(data, ZONE, country), 'links').lists[0];
    expect(link.title).toBe('ann@example.test');
    expect(link.facts.find((f) => f.key === 'return_path')?.value).toBe('/account/');
    expect(link.facts.find((f) => f.key === 'used')?.value).toBe('Not used');
    expect(link.facts.find((f) => f.key === 'removed')?.value).toBe(at('2026-10-23T12:00:00Z'));
  });

  it.each([
    ['requests', { requests: [] }],
    ['links', { email_links: [] }],
  ])('says so when there are no %s', (id, change) => {
    const g = group(keptGroups({ ...data, ...change }, ZONE, country), id);
    expect(g.lists).toEqual([]);
    expect(g.empty).not.toBe('');
  });
});

describe('shownZone', () => {
  it.each([
    ['a saved IANA zone', 'Pacific/Auckland', 'Pacific/Auckland'],
    ['a region, which is not a zone', 'US East', 'Europe/Berlin'],
    ['Unknown', 'Unknown', 'Europe/Berlin'],
  ])('shows times in %s', (_name, saved, want) => {
    expect(shownZone(saved, 'Europe/Berlin')).toBe(want);
  });
});

/** recorded is a fetch that answers status with body, keeping each call. */
function recorded(status: number, body: unknown) {
  const calls: { url: string; init: RequestInit | undefined }[] = [];
  const fetchFn = (async (url: string, init?: RequestInit) => {
    calls.push({ url, init });
    return new Response(JSON.stringify(body), { status });
  }) as typeof fetch;
  return { calls, fetchFn };
}

describe('fetchMyData', () => {
  it('asks with credentials and returns what is kept', async () => {
    const { calls, fetchFn } = recorded(200, data);
    expect(await fetchMyData('https://api.example/', fetchFn)).toEqual(data);
    expect(calls[0]).toEqual({ url: 'https://api.example/v1/me/data', init: { credentials: 'include' } });
  });

  it('returns null when nobody is signed in', async () => {
    expect(await fetchMyData('https://api.example', recorded(401, { error: 'not signed in' }).fetchFn)).toBeNull();
  });

  it('fails on any other answer', async () => {
    await expect(fetchMyData('https://api.example', recorded(500, { error: 'boom', request_id: 'r1' }).fetchFn)).rejects.toThrow('boom (ref r1)');
  });
});
