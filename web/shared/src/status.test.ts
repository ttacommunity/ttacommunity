import { describe, expect, it } from 'vitest';
import { statusClass } from './status';

describe('statusClass', () => {
  it.each([
    { name: 'muted is the status alone', tone: 'muted' as const, want: 'status' },
    { name: 'gold adds its tone', tone: 'gold' as const, want: 'status status-gold' },
    { name: 'good adds its tone', tone: 'good' as const, want: 'status status-good' },
    { name: 'warn adds its tone', tone: 'warn' as const, want: 'status status-warn' },
    { name: 'bad adds its tone', tone: 'bad' as const, want: 'status status-bad' },
  ])('$name', ({ tone, want }) => {
    expect(statusClass(tone)).toBe(want);
  });
});
