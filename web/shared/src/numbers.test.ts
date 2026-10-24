import { describe, expect, it } from 'vitest';
import { signed } from './numbers';

describe('signed', () => {
  it.each([
    ['a rise', 2.1, 1, '+2.1'],
    ['a fall', -3, 1, '-3.0'],
    ['zero', 0, 1, '0.0'],
    ['minus zero', -0, 1, '0.0'],
    ['a rise that rounds to zero', 0.04, 1, '0.0'],
    ['a fall that rounds to zero', -0.004, 2, '0.00'],
    ['a rise to two places', 2.47, 2, '+2.47'],
    ['a fall in whole points', -0.6, 0, '-1'],
    ['a small move in whole points', 0.4, 0, '0'],
  ])('writes %s', (_name, value, decimals, want) => {
    expect(signed(value, decimals)).toBe(want);
  });
});
