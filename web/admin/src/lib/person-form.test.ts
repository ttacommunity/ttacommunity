import { describe, expect, it } from 'vitest';
import { emptyPerson, entrantToAdd, memberToAdd, personChoices, wildcardChoices, type Candidate, type PersonDetails } from './person-form';

const candidate = (c: Partial<Candidate> & { name: string }): Candidate => ({
  member: null,
  player: '',
  discord_name: '',
  email: '',
  country: null,
  zone: null,
  banned: false,
  ...c,
});
const anna = candidate({ member: 101, name: 'anna', discord_name: 'anna#1', country: 'DE', zone: 'US East' });
const ann = candidate({ name: 'Ann', player: 'Ann', country: 'GB' });
const bea = candidate({ member: 102, name: 'Bea', zone: 'Unknown', banned: true });
const cy = candidate({ member: 103, name: 'cy', player: 'Cyrus' });

describe('personChoices', () => {
  it.each([
    ['each one found, then adding what was typed', [anna, ann], ' an ', [['anna', 'member · DE', false], ['Ann', 'in the ratings · GB', false], ['Add “an” as a new player', 'not in the ratings', false]]],
    ['nobody new when one found has the name', [anna, ann], 'ANN', [['anna', 'member · DE', false], ['Ann', 'in the ratings · GB', false]]],
    ['nobody new when one found has it as their ratings spelling', [cy], 'cyrus', [['cy', 'member', false]]],
    ['a banned member, not to be picked', [bea], 'bea', [['Bea', 'member · banned', true]]],
    ['nobody new for one letter', [], 'a', []],
    ['nobody new by the name of an entrant, said to be entered', [], 'ZED', [['Zed', 'entered already', true]]],
  ])('%s', (_, found, typed, want) => {
    expect(personChoices(found, typed, { entered: ['Zed'], membersTaken: false }).map((c) => [c.label, c.detail, c.disabled])).toEqual(want);
  });

  it('offers no member when adding members, only the players and someone new', () => {
    expect(personChoices([anna, ann], 'an', { entered: [], membersTaken: true }).map((c) => [c.label, c.detail, c.disabled])).toEqual([
      ['anna', 'member already · DE', true],
      ['Ann', 'in the ratings · GB', false],
      ['Add “an” as a new player', 'not in the ratings', false],
    ]);
  });

  it('fills the form with what is known: a member, a ratings player, someone new', () => {
    const [member, player, someone] = personChoices([anna, ann], 'an', { entered: [], membersTaken: false }).map((c) => c.fill);
    expect(member).toEqual({ member: 101, name: 'anna', discord_name: 'anna#1', email: '', country: 'DE', zone: 'US East' });
    expect(player).toEqual({ ...emptyPerson, name: 'Ann', country: 'GB' });
    expect(someone).toEqual({ ...emptyPerson, name: 'an' });
  });
});

const form = (f: Partial<PersonDetails>): PersonDetails => ({ ...emptyPerson, ...f });

describe('entrantToAdd', () => {
  it.each([
    ['a member by id, with the details given', form({ member: 101, name: 'anna', discord_name: ' anna#1 ', zone: 'US East' }), false, { member: 101, discord_name: 'anna#1', zone: 'US East', status: 'active' }],
    ['someone new by name, their zone Unknown', form({ name: ' Zed ', email: 'zed@example.test', country: ' nz ' }), false, { name: 'Zed', email: 'zed@example.test', country: 'nz', zone: 'Unknown', status: 'active' }],
    ['not confirmed yet', form({ name: 'Zed' }), true, { name: 'Zed', zone: 'Unknown', status: 'pending' }],
  ])('%s', (_, f, pending, want) => {
    expect(entrantToAdd(f, pending)).toEqual(want);
  });
});

describe('memberToAdd', () => {
  it.each([
    ['a player the ratings know, by their spelling, with the details given', form({ name: 'Ann', country: 'GB', zone: 'Europe West' }), { name: 'Ann', country: 'GB', zone: 'Europe West' }],
    ['someone new, trimmed, empty details left out', form({ name: ' Zed ', discord_name: ' zed#1 ', email: ' ' }), { name: 'Zed', discord_name: 'zed#1', zone: 'Unknown' }],
  ])('%s', (_, f, want) => {
    expect(memberToAdd(f)).toEqual(want);
  });
});

describe('wildcardChoices', () => {
  const outsiders = [
    { id: 7, name: 'Cy' },
    { id: 8, name: 'Cyril' },
    { id: 9, name: 'Dee' },
  ];
  const cyrus = candidate({ name: 'Cyrus', player: 'Cyrus' });
  it.each([
    ['the entrants not in the round first, then the people found', [cyrus], 'cy', [['Cy', 'entrant', false], ['Cyril', 'entrant', false], ['Cyrus', 'in the ratings', false]]],
    ['someone new', [], 'Quill', [['Add “Quill” as a new player', 'not in the ratings', false]]],
    ['an entrant in the round, who cannot be picked', [], 'zed', [['Zed', 'entered already', true]]],
    ['nothing for no text', [cyrus], ' ', []],
  ])('%s', (_name, found, typed, want) => {
    expect(wildcardChoices(outsiders, found, typed, ['Cy', 'Cyril', 'Dee', 'Zed']).map((c) => [c.label, c.detail, c.disabled])).toEqual(want);
  });

  it('picks an entrant by id', () => {
    expect(wildcardChoices(outsiders, [], 'dee', []).map((c) => c.key)).toEqual(['entrant 9']);
  });
});
