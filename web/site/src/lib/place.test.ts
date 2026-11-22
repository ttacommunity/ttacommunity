import { describe, expect, it } from 'vitest';
import { placeTone } from './place';

describe('placeTone', () => {
  it.each([
    { name: 'first is gold', place: 1, want: 'gold' },
    { name: 'second is silver', place: 2, want: 'silver' },
    { name: 'third is bronze', place: 3, want: 'bronze' },
    { name: 'fourth has no medal', place: 4, want: 'other' },
    { name: 'a place past fourth has no medal', place: 5, want: 'other' },
  ])('$name', ({ place, want }) => {
    expect(placeTone(place)).toBe(want);
  });
});
