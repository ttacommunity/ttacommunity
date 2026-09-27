import { describe, expect, it } from 'vitest';
import { isCurrent, sections } from './nav';

describe('sections', () => {
  it('lists only the sections that exist', () => {
    expect(sections.map((s) => s.href)).toEqual(['/ratings/']);
  });
});

describe('isCurrent', () => {
  it('matches a section and every page under it', () => {
    expect(isCurrent('/ratings/', '/ratings/')).toBe(true);
    expect(isCurrent('/ratings', '/ratings/')).toBe(true);
    expect(isCurrent('/ratings/player/x/', '/ratings/')).toBe(true);
  });

  it('does not match another section or a shared prefix', () => {
    expect(isCurrent('/', '/ratings/')).toBe(false);
    expect(isCurrent('/ratingsx/', '/ratings/')).toBe(false);
    expect(isCurrent('/design/', '/ratings/')).toBe(false);
  });
});
