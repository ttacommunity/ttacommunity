import { describe, expect, it } from 'vitest';
import { pickSummary } from './multi-pick';

describe('pickSummary', () => {
  const options = [
    { value: 'UC1', label: 'Invented Casts' },
    { value: 'UC2', label: 'Other Caster' },
  ];
  it.each([
    ['nothing ticked', [], 'All creators'],
    ['one ticked', ['UC2'], 'Other Caster'],
    ['several ticked', ['UC1', 'UC2'], '2 creators'],
  ] as const)('%s', (_, ticked, want) => {
    expect(pickSummary(options, ticked, 'All creators', 'creators')).toBe(want);
  });
});
