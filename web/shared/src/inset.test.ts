import { describe, expect, it } from 'vitest';
import { insetClass } from './inset';

describe('insetClass', () => {
  it.each([
    { name: 'a block is the box alone', form: 'block' as const, want: 'inset' },
    { name: 'figures lie in a row', form: 'figures' as const, want: 'inset inset-figures' },
    { name: 'a line is one short muted line', form: 'line' as const, want: 'inset inset-line' },
    { name: 'a row puts what it is beside its figures or actions', form: 'row' as const, want: 'inset inset-row' },
  ])('$name', ({ form, want }) => {
    expect(insetClass(form)).toBe(want);
  });
});
