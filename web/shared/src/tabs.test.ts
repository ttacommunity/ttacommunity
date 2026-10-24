import { describe, expect, it } from 'vitest';
import { hashTab, shownTab } from './tabs';

describe('shownTab', () => {
  it.each([
    ['the chosen tab while it is offered', ['standings', 'pairings', 'players'], 'players', 'players'],
    ['the first tab when none was chosen', ['standings', 'pairings', 'players'], null, 'standings'],
    ['the first tab when the chosen one is no longer offered', ['players'], 'pairings', 'players'],
  ] as const)('shows %s', (_name, offered, chosen, want) => {
    expect(shownTab(offered, chosen)).toBe(want);
  });
});

describe('hashTab', () => {
  it.each([
    ['the tab the hash names', ['standings', 'pairings', 'players'], '#pairings', 'pairings'],
    ['none without a hash', ['standings', 'pairings', 'players'], '', null],
    ['none for a tab not offered', ['players'], '#pairings', null],
    ['none for a hash naming no tab', ['standings', 'pairings', 'players'], '#Pairings', null],
  ] as const)('chooses %s', (_name, offered, hash, want) => {
    expect(hashTab(offered, hash)).toBe(want);
  });
});
