import { describe, expect, it } from 'vitest';
import { challengePicture, challengeSets, groupLabel } from './challenges';

const challenge = (id: string, set: 'original' | 'expansion' | 'mix', group: string) => ({ id, set, group });

describe('challengeSets', () => {
  it("puts the sets in the game's order, each group where its first challenge is, and the challenges as listed", () => {
    const list = [
      challenge('mix1', 'mix', 'mix'),
      challenge('a1', 'original', 'classic'),
      challenge('b1', 'original', 'epic'),
      challenge('a2', 'original', 'classic'),
      challenge('x1', 'expansion', 'gods'),
    ];
    const sets = challengeSets(list);
    expect(sets.map((s) => s.id)).toEqual(['original', 'expansion', 'mix']);
    expect(sets[0].groups.map((g) => [g.id, g.challenges.map((c) => c.id)])).toEqual([
      ['classic', ['a1', 'a2']],
      ['epic', ['b1']],
    ]);
  });

  it('leaves out a set with no challenges', () => {
    expect(challengeSets([challenge('a1', 'original', 'classic')]).map((s) => s.id)).toEqual(['original']);
  });
});

describe('groupLabel', () => {
  it.each([
    ['a group the game names', 'time_travellers', 'Time Travelers'],
    ['a group it does not know', 'new_group', 'New group'],
  ])('names %s', (_name, group, want) => {
    expect(groupLabel(group)).toBe(want);
  });
});

describe('challengePicture', () => {
  it("is the challenge's picture in the site's images", () => {
    expect(challengePicture('epic1_exp')).toBe('/img/challenges/epic1_exp.jpg');
  });
});
