import { describe, expect, it } from 'vitest';
import type { AdminLogin } from './admin-api';
import { detailsHint, howLinked, requestHint, signInGroups, standing } from './sign-ins';

const login = (id: number, extra: Partial<AdminLogin> = {}): AdminLogin => ({
  id,
  provider: 'discord',
  username: `player_${id}`,
  display_name: '',
  member_id: null,
  member: '',
  linked_how: null,
  signed_in_at: '2026-10-18T12:00:00Z',
  auto_link: true,
  request: null,
  refused: null,
  ...extra,
});
const asking = (member: string, memberDiscord: string) => ({
  id: 20,
  name: 'ann',
  country: '',
  zone: '',
  member,
  member_discord: memberDiscord,
  member_country: '',
  member_zone: '',
  requested_at: '2026-10-18T12:01:00Z',
});

describe('requestHint', () => {
  it.each([
    ['a name no member has: approving adds one', asking('', ''), 'new'],
    ['a member with no Discord name recorded', asking('ann', ''), 'unknown'],
    ['the recorded name, in another case and with an @', asking('ann', '@Player_1'), 'same'],
    ['another recorded name', asking('ann', 'someone_else'), 'differs'],
  ])('%s', (_name, request, want) => {
    expect(requestHint(request, 'player_1').kind).toBe(want);
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
    ['unlinked by an admin', login(6, { auto_link: false }), { text: 'unlinked by an admin', detail: 'not linked by Discord name again' }],
    ['unlinked, then refused', login(6, { auto_link: false, refused: { name: 'cat', reason: '' } }), { text: 'asked for cat, refused', detail: '' }],
  ])('%s', (_name, l, want) => {
    expect(standing(l)).toEqual(want);
  });
});

describe('howLinked', () => {
  it.each([
    ['by Discord name', 'match', 'linked by Discord name'],
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
