import { describe, expect, it } from 'vitest';
import { count, score, signed } from './numbers';

describe('count', () => {
  it.each([
    ['in an English browser', 1234567, 'en-US', '1,234,567'],
    ['in a German browser', 1234567, 'de-DE', '1.234.567'],
    ['below a thousand', 999, 'en-US', '999'],
  ])('writes a count %s', (_name, n, locale, want) => {
    expect(count(n, locale)).toBe(want);
  });

  it("writes a count in the browser's locale when given none", () => {
    expect(count(1234567)).toBe(new Intl.NumberFormat().format(1234567));
  });
});

describe('score', () => {
  it.each([
    ['a whole score', 197, '197'],
    ['a negative score', -2, '-2'],
    ['a half', 12.5, '12.5'],
    ['more decimals, to one place', 12.75, '12.8'],
  ])('writes %s', (_name, n, want) => {
    expect(score(n)).toBe(want);
  });
});

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
