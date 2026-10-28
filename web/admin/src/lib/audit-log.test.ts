import { describe, expect, it } from 'vitest';
import { auditBy, auditPath, auditSummary, dayStart, defaultView, detailText } from './audit-log';

describe('audit log', () => {
  it.each([
    ['UTC', '2026-10-28', '2026-10-28T00:00:00.000Z'],
    ['Australia/Sydney', '2026-10-28', '2026-10-27T13:00:00.000Z'],
    ['Australia/Sydney', '2026-04-05', '2026-04-04T13:00:00.000Z'],
    ['Australia/Sydney', '2026-04-06', '2026-04-05T14:00:00.000Z'],
    ['Europe/Berlin', '2026-03-29', '2026-03-28T23:00:00.000Z'],
    ['Europe/Berlin', '2026-03-30', '2026-03-29T22:00:00.000Z'],
    ['America/Los_Angeles', '2026-11-01', '2026-11-01T07:00:00.000Z'],
    ['America/Los_Angeles', '2026-11-02', '2026-11-02T08:00:00.000Z'],
  ])('starts the day in %s on %s at %s', (zone, day, want) => {
    expect(dayStart(day, zone)).toBe(want);
  });

  it.each([
    ['Australia/Sydney', '2026-10-28T01:13:00Z', '2026-10-22', '2026-10-28'],
    ['America/Los_Angeles', '2026-10-28T01:13:00Z', '2026-10-21', '2026-10-27'],
  ])('opens on the last seven days in %s at %s', (zone, now, from, to) => {
    expect(defaultView(new Date(now), zone)).toEqual({ action: '', text: '', from, to, page: 1 });
  });

  it.each([
    [
      'the default week',
      { action: '', text: '', from: '2026-10-22', to: '2026-10-28', page: 1 },
      '/api/audit?from=2026-10-21T13%3A00%3A00.000Z&to=2026-10-28T13%3A00%3A00.000Z&page=1',
    ],
    [
      'an action, a search and a page',
      { action: 'member.set', text: ' discord ', from: '2026-10-28', to: '2026-10-28', page: 3 },
      '/api/audit?action=member.set&q=discord&from=2026-10-27T13%3A00%3A00.000Z&to=2026-10-28T13%3A00%3A00.000Z&page=3',
    ],
    ['no days at all', { action: '', text: '', from: '', to: '', page: 1 }, '/api/audit?page=1'],
    ['a from day after the to day', { action: '', text: '', from: '2026-10-28', to: '2026-10-27', page: 1 }, null],
  ])('asks for %s', (_, view, want) => {
    expect(auditPath(view, 'Australia/Sydney')).toBe(want);
  });

  it.each([
    ['an admin', { admin: 'Owner', by_player: false, detail: '{}' }, 'Owner'],
    ['a player, with their account', { admin: '', by_player: true, detail: '{"account": 13}' }, 'A player (account 13)'],
    ['a player whose account is not recorded', { admin: '', by_player: true, detail: '{}' }, 'A player'],
    ['someone no longer in admin', { admin: '', by_player: false, detail: '{}' }, 'Someone no longer in admin'],
  ])('names %s', (_, entry, want) => {
    expect(auditBy(entry)).toBe(want);
  });

  it.each([
    ['an empty object', '{}', ''],
    ['null', 'null', ''],
    ['fields', '{"set": ["discord_name"], "member": 622}', 'set: discord_name · member: 622'],
    ['text that is not JSON', 'not json', 'not json'],
  ])('writes the detail of %s', (_, detail, want) => {
    expect(detailText(detail)).toBe(want);
  });

  it.each([
    [{ total: 0, page: 1, pages: 1 }, 'No entries match.'],
    [{ total: 1, page: 1, pages: 1 }, '1 entry'],
    [{ total: 79, page: 2, pages: 2 }, '79 entries · page 2 of 2'],
    [{ total: 12, page: 1, pages: 1 }, '12 entries'],
  ])('sums up %o as %s', (page, want) => {
    expect(auditSummary(page)).toBe(want);
  });
});
