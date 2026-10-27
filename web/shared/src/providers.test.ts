import { describe, expect, it } from 'vitest';
import { hasOwnName, isOpenID, PROVIDER_NAMES } from './providers';

describe('PROVIDER_NAMES', () => {
  it.each([
    ['discord', 'Discord'],
    ['email', 'Email'],
    ['microsoft', 'Microsoft'],
    ['google', 'Google'],
  ] as const)('names %s as %s', (provider, want) => {
    expect(PROVIDER_NAMES[provider]).toBe(want);
  });
});

describe('isOpenID', () => {
  it.each([
    ['discord', false],
    ['email', false],
    ['microsoft', true],
    ['google', true],
  ] as const)('says whether %s signs in through OpenID: %s', (provider, want) => {
    expect(isOpenID(provider)).toBe(want);
  });
});

describe('hasOwnName', () => {
  it.each([
    ['a Microsoft account with a name', 'microsoft', 'Player One', true],
    ['a Microsoft account without one', 'microsoft', 'Microsoft account', false],
    ['a Google account without one', 'google', 'Google account', false],
    ["a Google account named as Microsoft's nameless one", 'google', 'Microsoft account', true],
    ['a Discord username', 'discord', 'player_1', true],
  ] as const)('%s', (_name, provider, username, want) => {
    expect(hasOwnName(provider, username)).toBe(want);
  });
});
