import { describe, expect, it } from 'vitest';
import { noticeHeading } from './outcome';

describe('noticeHeading', () => {
  it.each([
    ['done', 'Done'],
    ['refused', 'Not done'],
    ['failed', 'Could not load'],
  ] as const)('names a %s outcome in words', (kind, want) => {
    expect(noticeHeading(kind)).toBe(want);
  });
});
