import { describe, expect, it } from 'vitest';
import {
  accountHref,
  draftChanges,
  accountState,
  backLabel,
  backPath,
  banNotice,
  askToBeLinked,
  changeMyDetails,
  countryOptions,
  addWayInHref,
  addEmail,
  deleteAccount,
  deleteQuestion,
  removeWayIn,
  signedInAs,
  waysIn,
  fetchCountries,
  fetchMe,
  isFirstSignIn,
  sharedMe,
  navLabel,
  needsContact,
  showsAdminLink,
  rememberSignedIn,
  saveOutcome,
  seemsSignedIn,
  signInHref,
  signInNotice,
  signOut,
  suggestedCountry,
  withoutSignInParam,
  zoneOptions,
  type Me,
  adminHref,
} from './account';

const discord = { provider: 'discord' as const, username: 'player_one', display_name: '', email: '' };
const email = { provider: 'email' as const, username: 'player@example.test', display_name: '', email: 'player@example.test' };
const base: Me = { provider: 'discord', username: 'player_one', display_name: '', logins: [discord], member: null, request: null, admin_role: null, country: null, zone: 'Unknown' };
const byEmail: Me = { ...base, provider: 'email', username: 'player@example.test', logins: [email] };
const microsoft = { provider: 'microsoft' as const, username: 'Player One', display_name: '', email: 'player@example.test' };
const unnamed = { ...microsoft, username: 'Microsoft account', email: '' };
const byMicrosoft: Me = { ...base, provider: 'microsoft', username: 'Player One', logins: [microsoft] };
const google = { provider: 'google' as const, username: 'Player One', display_name: '', email: 'player@example.test' };
const byGoogle: Me = { ...base, provider: 'google', username: 'Player One', logins: [google] };
const ann = { name: 'ann', player: 'Ann', banned: false, ban_reason: '' };
const asked = (status: 'pending' | 'approved' | 'refused', reason = '') => ({ name: 'ann', status, reason, requested_at: '2026-10-18T12:00:00Z' });

describe('accountState', () => {
  it.each([
    ['nobody signed in', null, { kind: 'signed-out' }],
    ['a sign-in with no request', base, { kind: 'unlinked' }],
    ['a request waiting', { ...base, request: asked('pending') }, { kind: 'waiting', name: 'ann' }],
    ['a refusal with its reason', { ...base, request: asked('refused', 'Message us') }, { kind: 'refused', name: 'ann', reason: 'Message us' }],
    ['an approval later undone', { ...base, request: asked('approved') }, { kind: 'unlinked' }],
    ['a member who has played', { ...base, member: ann }, { kind: 'linked', member: ann, playerHref: '/ratings/player/?name=Ann' }],
    ['a member who has not played', { ...base, member: { ...ann, player: '' } }, { kind: 'linked', member: { ...ann, player: '' }, playerHref: null }],
  ])('%s', (_name, me, want) => {
    expect(accountState(me)).toEqual(want);
  });
});

describe('showsAdminLink', () => {
  it.each([
    ['nobody signed in', null, false],
    ['a player', base, false],
    ['an organiser', { ...base, admin_role: 'organiser' as const }, true],
    ['an admin', { ...base, admin_role: 'admin' as const }, true],
  ])('%s', (_name, me, want) => {
    expect(showsAdminLink(me)).toBe(want);
  });
});

describe('navLabel', () => {
  it.each([
    ['nobody signed in', null, 'Sign in'],
    ['a linked member, by their TTA username', { ...base, display_name: 'Player One', member: ann }, 'ann'],
    ['an unlinked sign-in with a display name', { ...base, display_name: 'Player One' }, 'Player One'],
    ['an unlinked sign-in without one', base, 'player_one'],
    ['an unlinked email sign-in, never by its address', byEmail, 'Your account'],
    ['an email sign-in to an account with Discord, by the Discord name', { ...byEmail, logins: [{ ...discord, display_name: 'Player One' }, email] }, 'Player One'],
    ['a Microsoft sign-in, by its name', byMicrosoft, 'Player One'],
    ['a Microsoft sign-in with no name', { ...byMicrosoft, username: 'Microsoft account', logins: [unnamed] }, 'Your account'],
    ['an email sign-in to an account with Microsoft, by its name', { ...byEmail, logins: [email, microsoft] }, 'Player One'],
    ['a Google sign-in, by its name', byGoogle, 'Player One'],
    ['a Google sign-in with no name', { ...byGoogle, username: 'Google account', logins: [{ ...google, username: 'Google account' }] }, 'Your account'],
    ['an email sign-in to an account with Google, by its name', { ...byEmail, logins: [email, google] }, 'Player One'],
  ])('%s', (_name, me, want) => {
    expect(navLabel(me)).toBe(want);
  });
});

describe('signedInAs', () => {
  it.each([
    ['by Discord, with a display name', { ...base, display_name: 'Player One' }, { heading: 'Signed in with Discord as Player One', detail: 'Discord username: player_one' }],
    ['by Discord, without one', base, { heading: 'Signed in with Discord as player_one', detail: '' }],
    ['by email', byEmail, { heading: 'Signed in by email as player@example.test', detail: '' }],
    ['by Microsoft', byMicrosoft, { heading: 'Signed in with Microsoft as Player One', detail: '' }],
    ['by Google', byGoogle, { heading: 'Signed in with Google as Player One', detail: '' }],
  ])('%s', (_name, me, want) => {
    expect(signedInAs(me)).toEqual(want);
  });
});

describe('waysIn', () => {
  it.each([
    ['Discord alone: email may be added, nothing removed', base, ['Discord: player_one'], [false], { discord: false, email: true, microsoft: true, google: true }],
    [
      'both, each removable',
      { ...base, logins: [{ ...discord, display_name: 'Player One' }, email] },
      ['Discord: player_one (Player One)', 'Email: player@example.test'],
      [true, true],
      { discord: false, email: false, microsoft: true, google: true },
    ],
    ['email alone: Discord may be added', byEmail, ['Email: player@example.test'], [false], { discord: true, email: false, microsoft: true, google: true }],
    ['Microsoft, with the email it gave', byMicrosoft, ['Microsoft: Player One (player@example.test)'], [false], { discord: true, email: true, microsoft: false, google: true }],
    ['Microsoft, with no name or email', { ...byMicrosoft, logins: [unnamed] }, ['Microsoft: Microsoft account'], [false], { discord: true, email: true, microsoft: false, google: true }],
    ['Google, with the email it gave', byGoogle, ['Google: Player One (player@example.test)'], [false], { discord: true, email: true, microsoft: true, google: false }],
    [
      'Google, with no name or email',
      { ...byGoogle, logins: [{ ...google, username: 'Google account', email: '' }] },
      ['Google: Google account'],
      [false],
      { discord: true, email: true, microsoft: true, google: false },
    ],
  ])('%s', (_name, me, labels, removable, canAdd) => {
    const got = waysIn(me);
    expect(got.ways.map((w) => w.label)).toEqual(labels);
    expect(got.ways.map((w) => w.removable)).toEqual(removable);
    expect(got.ways.map((w) => w.changeable)).toEqual(got.ways.map((w) => w.provider === 'email'));
    expect(got.canAdd).toEqual(canAdd);
  });
});

describe('addWayInHref', () => {
  it.each([
    ['discord' as const, 'https://api.example/v1/auth/discord?add=true'],
    ['microsoft' as const, 'https://api.example/v1/auth/microsoft?add=true'],
    ['google' as const, 'https://api.example/v1/auth/google?add=true'],
  ])('opens the api to add %s, coming back to the account page', (provider, want) => {
    expect(addWayInHref('https://api.example/', provider)).toBe(want);
  });
});

describe('signInHref', () => {
  it.each([
    ['discord' as const, 'https://api.example/v1/auth/discord?return=%2Fblitz%2F'],
    ['microsoft' as const, 'https://api.example/v1/auth/microsoft?return=%2Fblitz%2F'],
    ['google' as const, 'https://api.example/v1/auth/google?return=%2Fblitz%2F'],
  ])('opens the api, which sends the browser to %s and back to the page', (provider, want) => {
    expect(signInHref('https://api.example/', provider, '/blitz/')).toBe(want);
  });
});

describe('needsContact', () => {
  it.each([
    ['Discord', base, false],
    ['an email way in', byEmail, false],
    ['Microsoft with an email', byMicrosoft, false],
    ['Microsoft with none', { ...byMicrosoft, logins: [unnamed] }, true],
    ['Google with an email', byGoogle, false],
    ['Google with none', { ...byGoogle, logins: [{ ...google, email: '' }] }, true],
  ])('%s', (_name, me, want) => {
    expect(needsContact(me)).toBe(want);
  });
});

describe('signInNotice', () => {
  it.each([
    ['a sign-in that finished', '?signin=ok', null],
    ['nothing to say', '', null],
    ['one that did not', '?signin=failed', 'failed'],
    ['an account just deleted', '?signin=deleted', 'deleted'],
    ['Discord added', '?signin=added', 'added'],
    ['an email added', '?signin=email-added', 'added'],
    ['a Discord account with an account of its own', '?signin=taken', 'failed'],
    ['an account with another Discord', '?signin=has-kind', 'failed'],
    ['anything else', '?signin=other', null],
  ])('%s', (_name, search, want) => {
    expect(signInNotice(new URLSearchParams(search))?.kind ?? null).toBe(want);
  });
  it.each([
    ['added', 'discord', 'Discord'],
    ['added', 'microsoft', 'Microsoft'],
    ['taken', 'microsoft', 'Microsoft'],
    ['has-kind', 'microsoft', 'Microsoft'],
    ['added', 'google', 'Google'],
    ['taken', 'google', 'Google'],
    ['has-kind', 'google', 'Google'],
  ])('%s names the way in, %s', (outcome, way, name) => {
    expect(signInNotice(new URLSearchParams({ signin: outcome, way }))?.text).toContain(name);
  });
});

describe('withoutSignInParam', () => {
  it.each([
    ['the only parameter', '/account/', '?signin=ok', '/account/'],
    ['among others', '/blitz/', '?t=us_open&signin=ok&e=2026', '/blitz/?t=us_open&e=2026'],
  ])('drops it as %s', (_name, path, search, want) => {
    expect(withoutSignInParam(path, search)).toBe(want);
  });
});

/** memoryStore is a Storage that keeps values in a map; refusing makes every call throw, as a blocked store does. */
function memoryStore(refusing = false): Storage {
  const m = new Map<string, string>();
  const no = () => {
    throw new Error('blocked');
  };
  return {
    get length() {
      return m.size;
    },
    clear: () => m.clear(),
    key: (i) => [...m.keys()][i] ?? null,
    getItem: (k) => (refusing ? no() : (m.get(k) ?? null)),
    setItem: (k, v) => (refusing ? no() : void m.set(k, v)),
    removeItem: (k) => (refusing ? no() : void m.delete(k)),
  };
}

describe('the signed-in mark', () => {
  it('is kept after a sign-in and dropped after signing out', () => {
    const store = memoryStore();
    expect(seemsSignedIn(store)).toBe(false);
    rememberSignedIn(store, true);
    expect(seemsSignedIn(store)).toBe(true);
    rememberSignedIn(store, false);
    expect(seemsSignedIn(store)).toBe(false);
  });

  it('is never there without a store', () => {
    rememberSignedIn(null, true);
    expect(seemsSignedIn(null)).toBe(false);
  });

  it('is never there in a store that refuses', () => {
    const store = memoryStore(true);
    rememberSignedIn(store, true);
    expect(seemsSignedIn(store)).toBe(false);
  });
});

describe('zoneOptions', () => {
  it.each([
    ['a zone the browser knows', 'Europe/Berlin', ['Unknown', 'Europe/Berlin', 'Pacific/Auckland']],
    ['Unknown', 'Unknown', ['Unknown', 'Europe/Berlin', 'Pacific/Auckland']],
    ['a region an organiser set', 'US East', ['Unknown', 'US East', 'Europe/Berlin', 'Pacific/Auckland']],
  ])('offers Unknown, the zone held when the browser does not know it, then every zone it knows: %s', (_name, current, want) => {
    expect(zoneOptions(['Pacific/Auckland', 'Europe/Berlin'], current)).toEqual(want);
  });
});

describe('countryOptions', () => {
  const names = { of: (code: string) => ({ DE: 'Germany', NZ: 'New Zealand' })[code] } as Intl.DisplayNames;
  it.each([
    ['names, sorted by name', ['NZ', 'DE'], names, [{ code: 'DE', name: 'Germany' }, { code: 'NZ', name: 'New Zealand' }]],
    ['the code where the browser has no name', ['XK'], names, [{ code: 'XK', name: 'XK' }]],
    ['codes alone without names', ['NZ', 'DE'], null, [{ code: 'DE', name: 'DE' }, { code: 'NZ', name: 'NZ' }]],
  ])('%s', (_name, codes, displayNames, want) => {
    expect(countryOptions(codes, displayNames)).toEqual(want);
  });
});

/** recorded is a fetch that answers with status and body, keeping each request it was given. */
function recorded(status: number, body: unknown) {
  const calls: { url: string; init: RequestInit | undefined }[] = [];
  const fetchFn = (async (url: string, init?: RequestInit) => {
    calls.push({ url, init });
    return new Response(status === 204 ? null : JSON.stringify(body), { status });
  }) as typeof fetch;
  return { calls, fetchFn };
}

describe('fetchMe', () => {
  it('asks with credentials and returns who is signed in', async () => {
    const { calls, fetchFn } = recorded(200, base);
    expect(await fetchMe('https://api.example', fetchFn)).toEqual(base);
    expect(calls[0]).toEqual({ url: 'https://api.example/v1/me', init: { credentials: 'include' } });
  });

  it('returns null when nobody is signed in', async () => {
    expect(await fetchMe('https://api.example', recorded(401, { error: 'not signed in' }).fetchFn)).toBeNull();
  });

  it('fails on any other answer', async () => {
    await expect(fetchMe('https://api.example', recorded(500, { error: 'boom', request_id: 'r1' }).fetchFn)).rejects.toThrow('boom (ref r1)');
  });
});

describe('sharedMe', () => {
  it('asks the api once for every part of the page', async () => {
    const { calls, fetchFn } = recorded(200, base);
    await Promise.all([sharedMe('https://api.example', fetchFn), sharedMe('https://api.example', fetchFn)]);
    expect(calls).toHaveLength(1);
  });

  it('asks again after a failure', async () => {
    const { calls, fetchFn } = recorded(500, { error: 'boom' });
    await expect(sharedMe('https://api.example', fetchFn)).rejects.toThrow('boom');
    await expect(sharedMe('https://api.example', fetchFn)).rejects.toThrow('boom');
    expect(calls).toHaveLength(2);
  });
});

describe('fetchCountries', () => {
  it('reads the codes without credentials', async () => {
    const { calls, fetchFn } = recorded(200, ['DE', 'NZ']);
    expect(await fetchCountries('https://api.example', fetchFn)).toEqual(['DE', 'NZ']);
    expect(calls[0]).toEqual({ url: 'https://api.example/v1/countries', init: { credentials: 'omit' } });
  });
});

describe('the changes a player makes', () => {
  it.each([
    ['asking to be linked', (f: typeof fetch) => askToBeLinked('https://api.example', ' ann ', f), 'PUT', '/v1/me/link-request', { name: 'ann' }],
    ['changing their details', (f: typeof fetch) => changeMyDetails('https://api.example', { country: 'DE', zone: 'Europe/Berlin' }, f), 'PATCH', '/v1/me/details', { country: 'DE', zone: 'Europe/Berlin' }],
    ['signing out', (f: typeof fetch) => signOut('https://api.example', f), 'POST', '/v1/auth/sign-out', undefined],
    ['deleting their account', (f: typeof fetch) => deleteAccount('https://api.example', f), 'DELETE', '/v1/me', undefined],
    ['adding an email', (f: typeof fetch) => addEmail('https://api.example', ' Player@example.test ', f), 'POST', '/v1/me/logins/email', { email: 'Player@example.test' }],
    ['removing a way in', (f: typeof fetch) => removeWayIn('https://api.example', 'discord', f), 'DELETE', '/v1/me/logins/discord', undefined],
  ])('%s sends credentials and JSON', async (_name, call, method, path, body) => {
    const { calls, fetchFn } = recorded(204, null);
    expect(await call(fetchFn)).toEqual({ ok: true });
    expect(calls[0].url).toBe('https://api.example' + path);
    expect(calls[0].init?.method).toBe(method);
    expect(calls[0].init?.credentials).toBe('include');
    expect(calls[0].init?.body === undefined ? undefined : JSON.parse(String(calls[0].init.body))).toEqual(body);
  });

  it("returns the api's refusal", async () => {
    expect(await askToBeLinked('https://api.example', 'ann', recorded(409, { error: 'you are linked to a member already' }).fetchFn)).toEqual({
      ok: false,
      message: 'you are linked to a member already',
    });
  });

  it('says so when the api cannot be reached, rather than throwing', async () => {
    const unreachable = (() => Promise.reject(new TypeError('Failed to fetch'))) as typeof fetch;
    expect(await changeMyDetails('https://api.example', { country: '', zone: 'UTC' }, unreachable)).toEqual({
      ok: false,
      message: 'The site could not reach its server. Check your connection and try again.',
    });
  });
});

describe('banNotice', () => {
  const linked = (member: typeof ann) => accountState({ ...base, member });
  it.each([
    ['a sign-in not linked', accountState(base), null],
    ['a member not banned', linked(ann), null],
    ['a banned member, with the reason', linked({ ...ann, banned: true, ban_reason: 'Left games unfinished' }), 'Left games unfinished'],
    ['a banned member, no reason given', linked({ ...ann, banned: true }), ''],
  ])('%s', (_name, state, wantReason) => {
    const got = banNotice(state);
    expect(got === null ? null : got.reason).toBe(wantReason);
  });
});

describe('deleteQuestion', () => {
  it.each([
    ['a linked member, naming whose details are cleared', accountState({ ...base, member: ann }), 'ann'],
    ['a sign-in not linked', accountState({ ...base, request: asked('pending') }), null],
  ])('asks before an action that cannot be undone: %s', (_name, state, names) => {
    const q = deleteQuestion(state);
    expect(q.final).toBe(true);
    expect(names === null || (q.text ?? '').includes(names)).toBe(true);
  });
});

describe('suggestedCountry', () => {
  const players = [
    { name: 'Sample_One', country: 'NZ' },
    { name: 'Sample_Two', country: null },
  ];
  it.each([
    ["the picked player's country, in any case", 'sample_one', 'Europe/Berlin', 'UTC', 'NZ'],
    ['the chosen zone\'s country, for a player with none', 'Sample_Two', 'Europe/Berlin', 'UTC', 'DE'],
    ['the chosen zone\'s country, for a name not listed', 'Someone_New', 'Australia/Sydney', 'UTC', 'AU'],
    ["the browser's zone when the chosen one has no country", 'Someone_New', 'Unknown', 'Asia/Calcutta', 'IN'],
    ['nothing when neither zone has one', '', 'US East', 'UTC', null],
  ])('%s', (_name, typed, zone, browserZone, want) => {
    expect(suggestedCountry(players, typed, zone, browserZone)).toBe(want);
  });
});

describe('backPath', () => {
  it.each([
    ['a page', '/blitz/', '/blitz/'],
    ['a page with a query', '/ratings/player/?name=a+b', '/ratings/player/?name=a+b'],
    ['nothing', null, null],
    ['another site', 'https://evil.example/', null],
    ['another site, scheme-relative', '//evil.example/', null],
    ['a backslash', '/\\evil.example', null],
    ['a fragment', '/ratings/#top', null],
    ['the account page itself', '/account/', null],
    ['the account page with a query', '/account/?x=1', null],
    ['something too long', '/' + 'a'.repeat(300), null],
  ])('%s', (_name, value, want) => {
    expect(backPath(value)).toBe(want);
  });
});

describe('accountHref', () => {
  it.each([
    ['from a page', '/blitz/', '?t=us_open&e=2026', '/account/?back=%2Fblitz%2F%3Ft%3Dus_open%26e%3D2026'],
    ['from the account page', '/account/', '?back=%2Fblitz%2F', '/account/'],
    ['dropping how a sign-in ended', '/ratings/', '?signin=ok', '/account/?back=%2Fratings%2F'],
  ])('%s', (_name, path, search, want) => {
    expect(accountHref(path, search)).toBe(want);
  });
});

describe('isFirstSignIn', () => {
  it.each([
    ['a first sign-in', '?signin=first&back=%2Fblitz%2F', true],
    ['a later one', '?signin=ok', false],
    ['none', '', false],
  ])('%s', (_name, search, want) => {
    expect(isFirstSignIn(new URLSearchParams(search))).toBe(want);
  });
});

describe('backLabel', () => {
  it.each([
    ['nothing changed', false, false, 'Back'],
    ['changes not saved', true, false, 'Back without saving'],
    ['saved', false, true, 'Back to where you were'],
  ])('%s', (_name, changed, saved, want) => {
    expect(backLabel(changed, saved)).toBe(want);
  });
});

describe('draftChanges', () => {
  const saved = { name: 'ann', country: 'NZ', zone: 'Pacific/Auckland' };
  it.each([
    ['nothing changed', saved, { name: false, details: false }],
    ['the name, ignoring spaces around it', { ...saved, name: ' annie ' }, { name: true, details: false }],
    ['spaces alone', { ...saved, name: ' ann ' }, { name: false, details: false }],
    ['the name cleared, which sends nothing', { ...saved, name: '  ' }, { name: false, details: false }],
    ['the country', { ...saved, country: '' }, { name: false, details: true }],
    ['the zone and the name', { ...saved, name: 'annie', zone: 'UTC' }, { name: true, details: true }],
  ])('%s', (_name, draft, want) => {
    expect(draftChanges(saved, draft)).toEqual(want);
  });
});

describe('saveOutcome', () => {
  const ok = { ok: true } as const;
  const no = { ok: false, message: 'Not signed in.' } as const;
  it.each([
    ['the request alone', ok, null, { kind: 'done', heading: 'Done', text: 'Sent your request to be linked.' }],
    ['the details alone', null, ok, { kind: 'done', heading: 'Done', text: 'Saved your country and time zone.' }],
    ['both', ok, ok, { kind: 'done', heading: 'Done', text: 'Sent your request to be linked, and saved your country and time zone.' }],
    ['the request refused', no, null, { kind: 'refused', heading: 'Not done', text: 'Not signed in.' }],
    ['the details refused', null, no, { kind: 'refused', heading: 'Not done', text: 'Not signed in.' }],
    [
      'the request sent, the details refused',
      ok,
      no,
      { kind: 'refused', heading: 'Not done', text: 'Sent your request to be linked, but your country and time zone were not saved: Not signed in.' },
    ],
    ['nothing sent', null, null, { kind: 'refused', heading: 'Not done', text: 'Nothing to save.' }],
  ] as const)('%s', (_name, asked, details, want) => {
    expect(saveOutcome(asked, details)).toEqual(want);
  });
});

describe('adminHref', () => {
  it.each([
    ['a challenge page, to its tab', '/challenges/epic1_exp/', 'https://admin.example/#challenges'],
    ['the highscores, to its tab', '/challenges/', 'https://admin.example/#challenges'],
    ["any other page, to admin's home", '/ratings/', 'https://admin.example'],
  ])('leads from %s', (_name, path, want) => {
    expect(adminHref('https://admin.example/', path)).toBe(want);
  });
});
