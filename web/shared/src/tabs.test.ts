import { describe, expect, it } from 'vitest';
import { shownTab } from './tabs';

describe('shownTab', () => {
  it.each([
    ['the chosen tab while it is offered', ['standings', 'pairings', 'players'], 'players', 'players'],
    ['the first tab when none was chosen', ['standings', 'pairings', 'players'], null, 'standings'],
    ['the first tab when the chosen one is no longer offered', ['players'], 'pairings', 'players'],
  ] as const)('shows %s', (_name, offered, chosen, want) => {
    expect(shownTab(offered, chosen)).toBe(want);
  });
});
