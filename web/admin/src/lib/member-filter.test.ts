import { describe, expect, it } from 'vitest';
import type { AdminMember } from './admin-api';
import { DEFAULT_STATUSES, membersMatching, membersWithStatus, memberStatuses, type MemberStatus } from './member-filter';

const member = (name: string, more: Partial<AdminMember> = {}): AdminMember => ({
  id: name.length,
  name,
  player: '',
  cge_user: null,
  discord_name: '',
  email: '',
  country: null,
  zone: 'Unknown',
  utc_offset: '',
  banned: false,
  ban_reason: '',
  banned_at: null,
  entries: 0,
  created_at: '2026-10-12T09:00:00Z',
  ...more,
});
const members = [
  member('anna', { discord_name: 'annie#1', country: 'DE' }),
  member('Bob', { player: 'Bobby', email: 'bob@example.test', country: 'US' }),
  member('Hannah'),
  member('zed_77', { country: 'NZ' }),
];

describe('membersMatching', () => {
  it.each([
    ['everyone for no text', ' ', ['anna', 'Bob', 'Hannah', 'zed_77']],
    ['a name holding the text, in the list order', 'ANN', ['anna', 'Hannah']],
    ['the ratings spelling', 'bobby', ['Bob']],
    ['a Discord name', 'annie', ['anna']],
    ['an email', 'example.test', ['Bob']],
    ['a country code', 'nz', ['zed_77']],
    ['a country only by its whole code', 'd', ['zed_77']],
    ['several, separated by commas', 'hann, zed', ['Hannah', 'zed_77']],
    ['nobody', 'quill', []],
  ])('%s', (_, typed, want) => {
    expect(membersMatching(members, typed).map((m) => m.name)).toEqual(want);
  });
});

describe('memberStatuses', () => {
  it.each<[string, Partial<AdminMember>, boolean, MemberStatus[]]>([
    ['a member a sign-in is linked to', {}, true, ['account']],
    ['a member in an edition', { entries: 2 }, false, ['entrant']],
    ['both', { entries: 1 }, true, ['account', 'entrant']],
    ['a member from the data alone, a flag and nothing else', { country: 'DE' }, false, ['data']],
  ])('%s', (_, more, linked, want) => {
    expect(memberStatuses(member('x', more), linked)).toEqual(want);
  });
});

describe('membersWithStatus', () => {
  // Bob has an account, Hannah is an entrant, anna is both; zed_77 is from the data.
  const people = [member('anna', { entries: 1 }), member('Bob'), member('Hannah', { entries: 3 }), member('zed_77', { country: 'NZ' })];
  const linked = new Set([people[0].id, people[1].id]);
  it.each<[string, ReadonlySet<MemberStatus>, string[]]>([
    ['by default, everyone but the data-only members', DEFAULT_STATUSES, ['anna', 'Bob', 'Hannah']],
    ['accounts only', new Set<MemberStatus>(['account']), ['anna', 'Bob']],
    ['entrants only', new Set<MemberStatus>(['entrant']), ['anna', 'Hannah']],
    ['the data-only members', new Set<MemberStatus>(['data']), ['zed_77']],
    ['everyone, in the list order', new Set<MemberStatus>(['account', 'entrant', 'data']), ['anna', 'Bob', 'Hannah', 'zed_77']],
    ['nobody for nothing ticked', new Set<MemberStatus>(), []],
  ])('%s', (_, statuses, want) => {
    expect(membersWithStatus(people, statuses, linked).map((m) => m.name)).toEqual(want);
  });
});
