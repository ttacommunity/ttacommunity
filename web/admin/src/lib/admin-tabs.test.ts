import { describe, expect, it } from 'vitest';
import { groupsFor, panelsFor, siteUrlFor, tabCounts, tabFrom, tabsFor, TABS } from './admin-tabs';

describe('admin tabs', () => {
  it.each([
    ['admin', ['tournaments', 'community', 'blitz', 'challenges', 'members', 'data', 'titles', 'admins', 'audit']],
    ['organiser', ['tournaments', 'blitz', 'challenges', 'members', 'data']],
  ] as const)('shows %s its tabs', (role, want) => {
    expect(tabsFor(role).map((t) => t.key)).toEqual(want);
  });

  it.each([
    ['#titles', 'admin', 'titles'],
    ['#community', 'admin', 'community'],
    ['#community', 'organiser', 'tournaments'],
    ['', 'admin', 'tournaments'],
    ['#nonsense', 'admin', 'tournaments'],
    ['#runs', 'admin', 'data'],
    ['#admins', 'organiser', 'tournaments'],
    ['#audit', 'admin', 'audit'],
    ['#audit', 'organiser', 'tournaments'],
    ['#data', 'organiser', 'data'],
    ['#blitz', 'organiser', 'blitz'],
    ['#challenges', 'organiser', 'challenges'],
    ['#signins', 'organiser', 'members'],
  ] as const)('opens %s for an %s on %s', (hash, role, want) => {
    expect(tabFrom(hash, role)).toBe(want);
  });

  it.each([
    ['community', 'admin', ['community']],
    ['community', 'organiser', ['community']],
    ['data', 'admin', ['data', 'runs', 'standings']],
    ['data', 'organiser', ['data', 'standings']],
    ['titles', 'admin', ['titles', 'badges']],
    ['tournaments', 'organiser', ['tournaments']],
    ['blitz', 'organiser', ['blitz']],
    ['challenges', 'organiser', ['challenges']],
    ['members', 'organiser', ['members']],
    ['admins', 'admin', ['admins']],
    ['audit', 'admin', ['audit']],
  ] as const)('shows the %s tab to an %s as its panels', (tab, role, want) => {
    expect(panelsFor(tab, role)).toEqual(want);
  });

  it.each([
    ['requests waiting', 2, 0, { members: 2 }],
    ['reports flagged', 0, 3, { blitz: 3 }],
    ['both', 1, 1, { members: 1, blitz: 1 }],
    ['none waiting', 0, 0, {}],
  ] as const)('counts on the tabs: %s', (_name, waiting, flagged, want) => {
    expect(tabCounts(waiting, flagged)).toEqual(want);
  });

  it.each([
    ['local dev IP', '127.0.0.1', 'http://127.0.0.1:4321'],
    ['localhost', 'localhost', 'http://127.0.0.1:4321'],
    ['dev admin subdomain', 'dev.admin.ttacommunity.org', 'https://dev.ttacommunity.org'],
    ['dev worker host', 'ttacommunity-dev.example.com', 'https://dev.ttacommunity.org'],
    ['production admin', 'admin.ttacommunity.org', 'https://ttacommunity.org'],
    ['empty or unknown', '', 'https://ttacommunity.org'],
  ] as const)('resolves site URL for %s: %s -> %s', (_name, host, want) => {
    expect(siteUrlFor(host)).toBe(want);
  });

  it.each([
    ['admin', 'admin', [['Tournaments', ['tournaments', 'community', 'blitz']], ['Challenges', ['challenges']], ['Players', ['members', 'titles']], ['Site', ['data', 'admins', 'audit']]]],
    ['organiser', 'organiser', [['Tournaments', ['tournaments', 'blitz']], ['Challenges', ['challenges']], ['Players', ['members']], ['Site', ['data']]]],
  ] as const)('groups the tabs %s sees, leaving out what they cannot', (_name, role, want) => {
    expect(groupsFor(tabsFor(role), {}).map((g) => [g.label, g.tabs.map((t) => t.key)])).toEqual(want);
  });

  it('puts every tab in a menu, so each has a link', () => {
    expect(groupsFor(TABS, {}).flatMap((g) => g.tabs.map((t) => t.key)).sort()).toEqual(TABS.map((t) => t.key).sort());
  });

  it('drops a group with no tab left', () => {
    expect(groupsFor(tabsFor('admin').filter((t) => t.key !== 'members' && t.key !== 'titles'), {}).map((g) => g.label)).toEqual(['Tournaments', 'Challenges', 'Site']);
  });

  it.each([
    ['nothing waiting', {}, [0, 0, 0, 0]],
    ['one tab waiting', { members: 3 }, [0, 0, 3, 0]],
    ['two tabs, two groups', { members: 1, blitz: 2 }, [2, 0, 1, 0]],
  ] as const)('totals each group\'s counts: %s', (_name, counts, want) => {
    expect(groupsFor(tabsFor('admin'), counts).map((g) => g.count)).toEqual(want);
  });
});
