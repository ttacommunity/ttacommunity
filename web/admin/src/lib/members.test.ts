import { describe, expect, it } from 'vitest';
import { isCountryCode, membersToEnter, type AdminMember } from './members';

const member = (id: number, name: string, banned = false): AdminMember => ({
  id,
  name,
  player: '',
  cge_user: null,
  discord_name: '',
  email: '',
  country: null,
  zone: 'UTC',
  utc_offset: '+00:00',
  banned,
  ban_reason: banned ? 'x' : '',
  banned_at: banned ? '2026-10-12T10:00:00Z' : null,
  entries: 0,
  created_at: '2026-10-12T09:00:00Z',
});

describe('membersToEnter', () => {
  it('lists members not yet in the edition and not banned, by name without case', () => {
    const members = [member(3, 'zed'), member(1, 'Ann'), member(2, 'bob'), member(4, 'Cy', true)];
    const entrants = [{ member: 2 }];
    expect(membersToEnter(members, entrants).map((m) => m.name)).toEqual(['Ann', 'zed']);
  });
});

describe('isCountryCode', () => {
  it.each([
    ['', true],
    ['de', true],
    ['US', true],
    [' nz ', true],
    ['D', false],
    ['Germany', false],
    ['1a', false],
  ])('judges %j as %s', (text, want) => {
    expect(isCountryCode(text)).toBe(want);
  });
});
