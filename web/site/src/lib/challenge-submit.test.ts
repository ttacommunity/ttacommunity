import { describe, expect, it } from 'vitest';
import { bestsOf, proofOwners, proofURL, reviewNotes, submission } from './challenge-submit';

const list = [
  { id: 'a', name: 'Alpha' },
  { id: 'b', name: 'Beta' },
];

describe('proofURL', () => {
  const message = 'https://discord.com/channels/358698681672925195/111/222';
  it.each([
    ['a message in the TtA server', message, message],
    ['the old domain, with spaces and a slash', ' https://discordapp.com/channels/358698681672925195/111/222/ ', message],
    ['nothing as no proof', '  ', ''],
    ['another server as wrong', 'https://discord.com/channels/1/111/222', null],
    ['another site as wrong', 'https://example.com', null],
  ])('reads %s', (_name, raw, want) => {
    expect(proofURL(raw)).toBe(want);
  });
});

describe('submission', () => {
  it('takes each filled row, the shared proof filling an empty one', () => {
    const got = submission(list, { a: { domination: '90', proof: '' }, b: { domination: ' ', proof: '' } }, 'https://discord.com/channels/358698681672925195/1/2');
    expect(got.entries).toEqual([{ challenge: 'a', domination: 90, proof_url: 'https://discord.com/channels/358698681672925195/1/2' }]);
    expect(got.errors).toEqual([]);
  });
  it.each([
    ['a domination that is not a whole number', { a: { domination: '9.5', proof: '' } }, 'Alpha: the domination must be a whole number.'],
    ['one out of bounds', { a: { domination: '10000', proof: '' } }, 'Alpha: the domination must be between -9999 and 9999.'],
    ['a proof from elsewhere', { a: { domination: '5', proof: 'https://example.com' } }, 'Alpha: the proof must be a message link in the TtA Discord.'],
  ])('refuses %s', (_name, rows, error) => {
    expect(submission(list, rows, '').errors).toEqual([error]);
  });
});

describe('reviewNotes', () => {
  const base = { domination: 100, proof: 'p', best: undefined, record: undefined, othersBest: undefined, reusedBy: [] as string[] };
  it.each([
    ['a new record with proof, and a first entry', base, ['good:★ New record!', 'good:First entry for this challenge']],
    ['a record without proof', { ...base, proof: '' }, ['good:★ Beats the record (add proof to make it official)', 'good:First entry for this challenge']],
    ['a gain on their best', { ...base, best: 80, record: 150 }, ['good:↑ +20 on your best']],
    ['not above their best', { ...base, best: 100, record: 150 }, ['warn:Not higher than your best']],
    ['a negative domination', { ...base, domination: -5, record: 150 }, ['warn:Negative dominations count as 0 in sums and ratings', 'good:First entry for this challenge']],
    ['far above everyone else', { ...base, record: 400, othersBest: 40, best: 10 }, ['good:↑ +90 on your best', 'warn:More than double everyone else\'s best. Typo?']],
    ['a proof someone else used', { ...base, record: 400, best: 10, reusedBy: ['Bob'] }, ['good:↑ +90 on your best', 'warn:Proof link already used by Bob']],
  ])('says %s', (_name, item, want) => {
    expect(reviewNotes(item).map((n) => `${n.kind}:${n.text}`)).toEqual(want);
  });
});

describe('proofOwners', () => {
  it('lists who else used each proof, ignoring case', () => {
    const owners = proofOwners([
      { player: 'Ann', proof_url: 'p1' },
      { player: 'Bob', proof_url: 'p1' },
      { player: 'ann', proof_url: 'p2' },
      { player: 'Cy', proof_url: null },
    ]);
    expect(owners('p1', 'ANN')).toEqual(['Bob']);
    expect(owners('p2', 'Ann')).toEqual([]);
    expect(owners('', 'Ann')).toEqual([]);
  });
});

describe('bestsOf', () => {
  const entries = [
    { challenge: 'a', player: 'Ann', domination: 50, proof_url: null, bugged: false },
    { challenge: 'a', player: 'ann', domination: 70, proof_url: 'p', bugged: false },
    { challenge: 'a', player: 'Bob', domination: 90, proof_url: null, bugged: false },
    { challenge: 'b', player: 'Bob', domination: 999, proof_url: null, bugged: true },
  ];
  it("gives a player's best on each challenge, ignoring case and bugged entries", () => {
    expect(bestsOf(entries, 'ANN')).toEqual(new Map([['a', 70]]));
  });
  it("gives everyone else's best", () => {
    expect(bestsOf(entries, 'Ann', true)).toEqual(new Map([['a', 90]]));
  });
});
