import { describe, expect, it } from 'vitest';
import { pageCount, entriesPath, viewLabel } from './challenge-entries';

describe('pageCount', () => {
  it.each([
    ['no entries', 0, 1],
    ['one short page', 99, 1],
    ['exactly a page', 100, 1],
    ['one over a page', 101, 2],
  ])('gives %s its pages', (_name, total, want) => {
    expect(pageCount(total, 100)).toBe(want);
  });
});


describe('entriesPath', () => {
  it.each([
    ['the first page of every entry', 1, 'all', '', '/api/challenges/entries?page=1'],
    ['a view, searched', 2, 'flagged', ' Ann B ', '/api/challenges/entries?page=2&view=flagged&q=Ann+B'],
  ] as const)('asks for %s', (_name, page, view, q, want) => {
    expect(entriesPath(page, view, q)).toBe(want);
  });
});

describe('viewLabel', () => {
  it('names a view with its count', () => {
    expect(viewLabel('unchecked', 4)).toBe('Verified, not checked yet (4)');
  });
});
