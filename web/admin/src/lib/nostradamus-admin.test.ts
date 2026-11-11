import { describe, expect, it } from 'vitest';
import { memberPath, outcomePath, outcomeText, outcomeValues } from './nostradamus-admin';

describe('outcomeValues', () => {
  it.each([
    ['a pick', 'pick', ' Ann ', ['Ann']],
    ['a tie of a pick', 'pick', 'Ann, Cy', ['Ann', 'Cy']],
    ['a set', 'set', 'Ann,Cy, ', ['Ann', 'Cy']],
    ['a number, whole', 'number', ' 3 ', ['3']],
    ['nothing, to clear it', 'set', '  ', []],
  ] as const)('reads %s', (_name, kind, text, want) => {
    expect(outcomeValues(kind, text)).toEqual(want);
  });
});

describe('outcomeText', () => {
  it.each([
    [['Ann', 'Cy'], 'Ann, Cy'],
    [null, ''],
  ])('writes %s as %s', (outcome, want) => {
    expect(outcomeText(outcome)).toBe(want);
  });
});

describe('paths', () => {
  it('names the edition and the question or participant', () => {
    expect(outcomePath('wc-2031', 5)).toBe('/api/nostradamus/wc-2031/questions/5/outcome');
    expect(memberPath('wc-2031', 8)).toBe('/api/nostradamus/wc-2031/participants/8/member');
  });
});
