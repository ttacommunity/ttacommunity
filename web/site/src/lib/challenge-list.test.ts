import { describe, expect, it } from 'vitest';
import { CHALLENGE_LIST, challengeTiles, opponentLabel, previewText, type ChallengeInfo } from './challenge-list';

describe('opponentLabel', () => {
  it.each([
    ['a named opponent by name', { level: 'hard', name: 'Napoleon' }, 'Napoleon'],
    ['an unnamed AI by its level', { level: 'hard', name: null }, 'Top AI'],
  ] as const)('calls %s', (_name, opponent, want) => {
    expect(opponentLabel(opponent)).toBe(want);
  });
});

describe('previewText', () => {
  const base: ChallengeInfo = { id: 'x', set: 'mix', group: 'mix', slot: 0, difficulty: 1, players: 2, name: 'X', description: [], rules: [], opponents: [] };
  it.each([
    ['the story when there is one', { ...base, description: ['Once.', 'Twice.'], rules: ['Win.'] }, 'Once. Twice.'],
    ['the rules otherwise', { ...base, rules: ['Win.', 'Fast.'] }, 'Win. Fast.'],
  ])('says %s', (_name, c, want) => {
    expect(previewText(c)).toBe(want);
  });
});

describe('CHALLENGE_LIST', () => {
  it('names each challenge once', () => {
    const ids = CHALLENGE_LIST.map((c) => c.id);
    expect(new Set(ids).size).toBe(ids.length);
  });
});

describe('challengeTiles', () => {
  it('keeps only what a tile needs', () => {
    expect(Object.keys(challengeTiles()[0]).sort()).toEqual(['group', 'id', 'name', 'players', 'set']);
  });
});
