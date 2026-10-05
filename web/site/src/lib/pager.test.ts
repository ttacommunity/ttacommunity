import { describe, expect, it } from 'vitest';
import { typedPage } from './pager';

describe('typedPage', () => {
  it('goes to the page typed', () => {
    expect(typedPage('3', 10)).toBe(3);
  });

  it('keeps a page typed out of range within the pages there are', () => {
    expect(typedPage('0', 10)).toBe(1);
    expect(typedPage('99', 10)).toBe(10);
  });

  it('goes to the first page for anything that is not a number', () => {
    expect(typedPage('', 10)).toBe(1);
    expect(typedPage('two', 10)).toBe(1);
  });
});
