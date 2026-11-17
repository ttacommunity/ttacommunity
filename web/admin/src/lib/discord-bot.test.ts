import { describe, expect, it } from 'vitest';
import { commandChanged, commandSettings, registration } from './discord-bot';

const when = (iso: string) => `[${iso}]`;
const RATING = { name: 'rating', description: 'A rating', on: true, reply: 'private' as const, format: 0, results: null };
const ROLL = { name: 'roll', description: 'Dice', on: false, reply: 'shared' as const, format: null, results: null };
const UPSETS = { name: 'upsets', description: 'Upsets', on: true, reply: 'always_shared' as const, format: 2, results: 8 };
const COMMANDS = [RATING, ROLL, UPSETS];

describe('discord bot', () => {
  it.each([
    ['never', { commands: COMMANDS, registered_at: null, registered: [] }, 'Not registered with Discord yet: the api registers the commands as it starts, when the bot is set up.'],
    ['all there', { commands: COMMANDS, registered_at: '2026-11-17T06:00:00Z', registered: ['rating', 'roll', 'upsets'] }, 'Registered with Discord [2026-11-17T06:00:00Z]: 3 commands.'],
    [
      'one missing',
      { commands: COMMANDS, registered_at: '2026-11-17T06:00:00Z', registered: ['rating', 'roll'] },
      'Registered with Discord [2026-11-17T06:00:00Z]: 2 commands. Not there yet: /upsets (the next api start registers them).',
    ],
  ])('says how the commands are registered: %s', (_, bot, want) => {
    expect(registration(bot, when)).toBe(want);
  });

  it.each([
    ['a command with a format', RATING, 'Private unless shared · Every format'],
    ['a command without settings beyond its reply', ROLL, 'Shared unless private'],
    ['a list', UPSETS, 'Always shared · 2-player · 8 rows'],
  ])('says a command\'s settings in a line: %s', (_, c, want) => {
    expect(commandSettings(c)).toBe(want);
  });

  it.each([
    ['unchanged', UPSETS, false],
    ['another reply', { ...UPSETS, reply: 'private' as const }, true],
    ['another format', { ...UPSETS, format: 0 }, true],
    ['other rows', { ...UPSETS, results: 3 }, true],
  ])('a command\'s settings can be saved when %s', (_, draft, want) => {
    expect(commandChanged(UPSETS, draft)).toBe(want);
  });
});
