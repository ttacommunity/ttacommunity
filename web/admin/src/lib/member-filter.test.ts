import { describe, expect, it } from 'vitest';
import type { AdminMember } from './admin-api';
import { membersMatching } from './member-filter';

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
