import { describe, expect, it } from 'vitest';
import { entryFlags, pageCount } from './challenge-entries';

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

describe('entryFlags', () => {
  const entry = { hidden: false, hidden_reason: '', bugged: false, checked: false, proof_url: null as string | null };
  it.each([
    ['nothing for a plain unproven entry', entry, []],
    ['hidden with its reason', { ...entry, hidden: true, hidden_reason: 'a duplicate' }, ['Hidden: a duplicate']],
    ['hidden without a reason', { ...entry, hidden: true }, ['Hidden']],
    ['bugged', { ...entry, bugged: true }, ['Bugged']],
    ['a proof not yet looked at', { ...entry, proof_url: 'https://discord.com/channels/1/2/3' }, ['Proof not checked']],
    ['a checked proof', { ...entry, proof_url: 'https://discord.com/channels/1/2/3', checked: true }, ['Proof checked']],
  ])('says %s', (_name, e, want) => {
    expect(entryFlags(e)).toEqual(want);
  });
});
