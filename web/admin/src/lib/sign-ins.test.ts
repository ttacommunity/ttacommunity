import { describe, expect, it } from 'vitest';
import type { AdminAccount } from './admin-api';
import { detailsHint, howLinked, requestHint, signInGroups, standing, who } from './sign-ins';

const way = (provider: 'discord' | 'email' | 'microsoft' | 'google', username: string, display_name = '', email = provider === 'email' ? username : '') => ({
  provider,
  username,
  display_name,
  email,
  signed_in_at: '2026-10-18T12:00:00Z',
});
const login = (id: number, extra: Partial<AdminAccount> = {}): AdminAccount => ({
  id,
  logins: [way('discord', `player_${id}`)],
  member_id: null,
  member: '',
  linked_how: null,
  signed_in_at: '2026-10-18T12:00:00Z',
  auto_link: true,
  request: null,
  refused: null,
  ...extra,
});
const asking = (member: string, memberDiscord: string, memberEmail = '') => ({
  id: 20,
  name: 'ann',
  country: '',
  zone: '',
  member,
  member_discord: memberDiscord,
  member_email: memberEmail,
  member_country: '',
  member_zone: '',
  requested_at: '2026-10-18T12:01:00Z',
});

describe('requestHint', () => {
  const discord = login(1);
  const email = login(1, { logins: [way('email', 'player@example.test')] });
  const both = login(1, { logins: [way('discord', 'player_1'), way('email', 'player@example.test')] });
  const microsoft = login(1, { logins: [way('microsoft', 'Player One', '', 'player@example.test')] });
  const unnamed = login(1, { logins: [way('microsoft', 'Microsoft account')] });
  const google = login(1, { logins: [way('google', 'Player One', '', 'player@example.test')] });
  it.each([
    ['a name no member has: approving adds one', asking('', ''), discord, 'new'],
    ['a member with no Discord name recorded', asking('ann', ''), discord, 'unknown'],
    ['the recorded name, in another case and with an @', asking('ann', '@Player_1'), discord, 'same'],
    ['another recorded name', asking('ann', 'someone_else'), discord, 'differs'],
    ['an email sign-in and a member with only a Discord name', asking('ann', 'player_1'), email, 'unknown'],
    ['the recorded email, in another case', asking('ann', '', ' Player@Example.test'), email, 'same'],
    ['another recorded email', asking('ann', '', 'other@example.test'), email, 'differs'],
    ['one of two the same', asking('ann', 'someone_else', 'player@example.test'), both, 'same'],
    ["Microsoft's email, the recorded one", asking('ann', '', 'player@example.test'), microsoft, 'same'],
    ["Microsoft's email, another", asking('ann', '', 'other@example.test'), microsoft, 'differs'],
    ['Microsoft with no email', asking('ann', '', 'player@example.test'), unnamed, 'unknown'],
    ["Google's email, the recorded one", asking('ann', '', 'player@example.test'), google, 'same'],
    ["Google's email, another", asking('ann', '', 'other@example.test'), google, 'differs'],
  ])('%s', (_name, request, account, want) => {
    expect(requestHint(request, account).kind).toBe(want);
  });
});

describe('who', () => {
  it.each([
    ['a Discord name alone', login(1), 'player_1'],
    ['with its display name', login(1, { logins: [way('discord', 'player_1', 'Player One')] }), 'player_1 (Player One)'],
    ['Discord and email', login(1, { logins: [way('discord', 'player_1'), way('email', 'player@example.test')] }), 'player_1 · player@example.test'],
    ['Microsoft, named as such', login(1, { logins: [way('microsoft', 'Player One', '', 'player@example.test')] }), 'Player One (Microsoft)'],
    ['Microsoft with no name', login(1, { logins: [way('microsoft', 'Microsoft account')] }), 'Microsoft account'],
    ['Google, named as such', login(1, { logins: [way('google', 'Player One', '', 'player@example.test')] }), 'Player One (Google)'],
    ['Google with no name', login(1, { logins: [way('google', 'Google account')] }), 'Google account'],
  ])('%s', (_name, account, want) => {
    expect(who(account)).toBe(want);
  });
});

describe('signInGroups', () => {
  const rows = [
    login(2, { request: asking('', '') }),
    login(1, { member_id: 3, member: 'ann', linked_how: 'match' }),
    login(4),
    login(5, { refused: { name: 'cat', reason: '' } }),
  ];
  const { waiting, byMember, unlinked } = signInGroups(rows);
  it('puts the sign-ins waiting for an answer first, in order', () => {
    expect(waiting.map((l) => l.id)).toEqual([2]);
  });
  it('finds a linked sign-in by its member', () => {
    expect(byMember.get(3)?.id).toBe(1);
    expect(byMember.size).toBe(1);
  });
  it('keeps the rest apart, in order', () => {
    expect(unlinked.map((l) => l.id)).toEqual([4, 5]);
  });
});

describe('standing', () => {
  it.each([
    ['never asked', login(4), { text: 'has not asked to be linked', detail: '' }],
    ['refused with a reason', login(5, { refused: { name: 'cat', reason: 'Not the same player' } }), { text: 'asked for cat, refused', detail: 'Not the same player' }],
    ['refused without one', login(5, { refused: { name: 'cat', reason: '' } }), { text: 'asked for cat, refused', detail: '' }],
    ['unlinked by an admin', login(6, { auto_link: false }), { text: 'unlinked by an admin', detail: 'not linked by Discord name or email again' }],
    ['unlinked, then refused', login(6, { auto_link: false, refused: { name: 'cat', reason: '' } }), { text: 'asked for cat, refused', detail: '' }],
  ])('%s', (_name, l, want) => {
    expect(standing(l)).toEqual(want);
  });
});

describe('howLinked', () => {
  it.each([
    ['by a recorded name or email', 'match', 'linked by Discord name or email'],
    ['by approval', 'approved', 'approved'],
    ['not linked', null, ''],
  ] as const)('%s', (_name, how, want) => {
    expect(howLinked(login(1, { linked_how: how }))).toBe(want);
  });
});

describe('detailsHint', () => {
  const r = asking('ann', '');
  it.each([
    ['nothing given', { ...r }, 'none'],
    ['given, for a member to be added', { ...r, member: '', country: 'NZ', zone: 'Pacific/Auckland' }, 'given'],
    ['the same as recorded', { ...r, country: 'NZ', zone: 'Pacific/Auckland', member_country: 'NZ', member_zone: 'Pacific/Auckland' }, 'same'],
    ['different from recorded', { ...r, country: 'NZ', zone: 'Pacific/Auckland', member_country: 'DE', member_zone: 'UTC' }, 'differs'],
    ['nothing recorded to compare', { ...r, country: 'NZ', zone: 'Pacific/Auckland', member_zone: 'Unknown' }, 'given'],
  ])('%s', (_name, request, want) => {
    expect(detailsHint(request).kind).toBe(want);
  });
});
