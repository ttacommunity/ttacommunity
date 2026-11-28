import { describe, expect, it } from 'vitest';
import { dataTabFrom, dataTabs, flaggedReports, groupsFor, panelsFor, siteUrlFor, tabCounts, tabFrom, tabsFor, TABS } from './admin-tabs';

describe('admin tabs', () => {
  it.each([
    ['admin', ['tournaments', 'community', 'blitz', 'challenges', 'nostradamus', 'members', 'data', 'movies', 'images', 'titles', 'admins', 'notifications', 'bot', 'audit']],
    ['organiser', ['tournaments', 'blitz', 'challenges', 'nostradamus', 'members', 'data', 'images']],
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
    ['#notifications', 'admin', 'notifications'],
    ['#bot', 'admin', 'bot'],
    ['#bot', 'organiser', 'tournaments'],
    ['#movies', 'admin', 'movies'],
    ['#movies', 'organiser', 'tournaments'],
    ['#notifications', 'organiser', 'tournaments'],
    ['#data', 'organiser', 'data'],
    ['#blitz', 'organiser', 'blitz'],
    ['#challenges', 'organiser', 'challenges'],
    ['#nostradamus', 'organiser', 'nostradamus'],
    ['#signins', 'organiser', 'members'],
    ['#images', 'organiser', 'images'],
    ['#images', 'admin', 'images'],
    ['#games', 'organiser', 'data'],
    ['#standings', 'admin', 'data'],
  ] as const)('opens %s for an %s on %s', (hash, role, want) => {
    expect(tabFrom(hash, role)).toBe(want);
  });

  it.each([
    ['community', 'admin', ['community']],
    ['community', 'organiser', ['community']],
    ['data', 'admin', ['data', 'runs', 'games', 'standings']],
    ['data', 'organiser', ['data', 'games', 'standings']],
    ['titles', 'admin', ['titles', 'badges']],
    ['tournaments', 'organiser', ['tournaments']],
    ['blitz', 'organiser', ['blitz']],
    ['challenges', 'organiser', ['challenges']],
    ['members', 'organiser', ['members']],
    ['admins', 'admin', ['admins']],
    ['audit', 'admin', ['audit']],
    ['notifications', 'admin', ['notifications']],
    ['movies', 'admin', ['movies']],
    ['images', 'organiser', ['images']],
  ] as const)('shows the %s tab to an %s as its panels', (tab, role, want) => {
    expect(panelsFor(tab, role)).toEqual(want);
  });

  it.each([
    ['admin', [['data', 'Summary'], ['runs', 'Runs'], ['games', 'Games'], ['standings', 'Standings check']]],
    ['organiser', [['data', 'Summary'], ['games', 'Games'], ['standings', 'Standings check']]],
  ] as const)("names the data tab's sub tabs %s sees, each with its address", (role, want) => {
    expect(dataTabs(role).map((t) => [t.hash, t.label])).toEqual(want);
  });

  it.each([
    ['#games', 'admin', 'games'],
    ['#runs', 'admin', 'runs'],
    ['#standings', 'organiser', 'standings'],
    ['#runs', 'organiser', 'data'],
    ['#data', 'admin', 'data'],
    ['', 'admin', 'data'],
  ] as const)('opens %s for an %s on the data sub tab %s', (hash, role, want) => {
    expect(dataTabFrom(hash, role)).toBe(want);
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
    ['no editions', [], 0],
    ['none flagged', [{ flagged: 0 }, { flagged: 0 }], 0],
    ['reports across editions', [{ flagged: 2 }, { flagged: 0 }, { flagged: 3 }], 5],
  ] as const)('counts the flagged reports: %s', (_name, editions, want) => {
    expect(flaggedReports(editions)).toBe(want);
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
    ['admin', 'admin', [['Tournaments', ['tournaments', 'community', 'blitz']], ['Bread & Circuses', ['challenges', 'nostradamus']], ['Players', ['members', 'titles']], ['Site', ['data', 'movies', 'images', 'admins', 'notifications', 'bot', 'audit']]]],
    ['organiser', 'organiser', [['Tournaments', ['tournaments', 'blitz']], ['Bread & Circuses', ['challenges', 'nostradamus']], ['Players', ['members']], ['Site', ['data', 'images']]]],
  ] as const)('groups the tabs %s sees, leaving out what they cannot', (_name, role, want) => {
    expect(groupsFor(tabsFor(role), {}).map((g) => [g.label, g.tabs.map((t) => t.key)])).toEqual(want);
  });

  it('puts every tab in a menu, so each has a link', () => {
    expect(groupsFor(TABS, {}).flatMap((g) => g.tabs.map((t) => t.key)).sort()).toEqual(TABS.map((t) => t.key).sort());
  });

  it('drops a group with no tab left', () => {
    expect(groupsFor(tabsFor('admin').filter((t) => t.key !== 'members' && t.key !== 'titles'), {}).map((g) => g.label)).toEqual(['Tournaments', 'Bread & Circuses', 'Site']);
  });

  it.each([
    ['nothing waiting', {}, [0, 0, 0, 0]],
    ['one tab waiting', { members: 3 }, [0, 0, 3, 0]],
    ['two tabs, two groups', { members: 1, blitz: 2 }, [2, 0, 1, 0]],
  ] as const)('totals each group\'s counts: %s', (_name, counts, want) => {
    expect(groupsFor(tabsFor('admin'), counts).map((g) => g.count)).toEqual(want);
  });
});
