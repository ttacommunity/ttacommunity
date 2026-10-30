import { describe, expect, it } from 'vitest';
import { isCircuit } from './circuits';

describe('isCircuit', () => {
  it.each([
    ['the Grand Slams', 'grand_slams', true],
    ['the ladders', 'ladders', true],
    ['a major', 'international', false],
    ['another group', 'quick_and_dirty', false],
  ])('says whether %s has a Hall of Fame page', (_name, slug, want) => {
    expect(isCircuit(slug)).toBe(want);
  });
});
