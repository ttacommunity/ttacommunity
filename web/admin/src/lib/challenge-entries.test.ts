import { describe, expect, it } from 'vitest';
import { entriesMatching, entriesPath, viewLabel } from './challenge-entries';

describe('entriesPath', () => {
  it.each([
    ['every entry', 'all', '/api/challenges/entries'],
    ['a view', 'flagged', '/api/challenges/entries?view=flagged'],
  ] as const)('asks for %s', (_name, view, want) => {
    expect(entriesPath(view)).toBe(want);
  });
});

describe('entriesMatching', () => {
  const entry = (id: number, player: string, challenge: string, challenge_name: string) => ({ id, player, challenge, challenge_name });
  const entries = [entry(1, 'anna', '1ai', 'First'), entry(2, 'Bob', '2ai', 'Second'), entry(3, 'Hannah', '1ai', 'First'), entry(4, 'zed_77', '3ai', 'Third')];
  it.each([
    ['everyone for no text', ' ', [1, 2, 3, 4]],
    ['a player holding the text, in the list order', 'ANN', [1, 3]],
    ['a challenge id', '2ai', [2]],
    ["a challenge's name", 'third', [4]],
    ['several, separated by commas', 'bob, zed', [2, 4]],
    ['nobody', 'quill', []],
  ])('%s', (_, typed, want) => {
    expect(entriesMatching(entries, typed).map((e) => e.id)).toEqual(want);
  });
});

describe('viewLabel', () => {
  it('names a view with its count', () => {
    expect(viewLabel('unchecked', 4)).toBe('Verified, not checked yet (4)');
  });
});
