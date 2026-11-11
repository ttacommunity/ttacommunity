import { describe, expect, it } from 'vitest';
import { chosenEdition, editionParam, nostradamusHref, participantHref, places, pointsText, shownName, valuesText } from './nostradamus';

const editions = [{ slug: 'wc-2032' }, { slug: 'wc-2031' }];

describe('chosenEdition', () => {
  it.each([
    ['the one asked for', 'wc-2031', 'wc-2031'],
    ['the newest when none is asked for', null, 'wc-2032'],
    ['the newest when the one asked for is not an edition', 'wc-1999', 'wc-2032'],
  ])('is %s', (_name, wanted, want) => {
    expect(chosenEdition(editions, wanted)).toBe(want);
  });

  it('is none without editions', () => {
    expect(chosenEdition([], 'wc-2031')).toBeNull();
  });
});

describe('editionParam', () => {
  it.each([
    ['?edition=wc-2031&name=Ann', 'wc-2031'],
    ['?edition=%20', null],
    ['', null],
  ])('reads %s', (search, want) => {
    expect(editionParam(search)).toBe(want);
  });
});

describe('addresses', () => {
  it('names the edition, and the participant', () => {
    expect(nostradamusHref('wc-2031')).toBe('/nostradamus/?edition=wc-2031');
    expect(participantHref('wc-2031', 'Ann B')).toBe('/nostradamus/participant/?edition=wc-2031&name=Ann+B');
  });
});

describe('shownName', () => {
  it.each([
    ['the member it is linked to', { name: 'ann', member: 'Ann' }, 'Ann'],
    ['the name it answered under, unlinked', { name: 'Dee', member: null }, 'Dee'],
  ])('is %s', (_name, p, want) => {
    expect(shownName(p)).toBe(want);
  });
});

describe('valuesText', () => {
  it.each([
    ['one name', ['Ann'], 'Ann'],
    ['several names', ['Ann', 'Cy'], 'Ann, Cy'],
    ['none yet', null, 'Not known yet'],
  ])('writes %s', (_name, values, want) => {
    expect(valuesText(values)).toBe(want);
  });
});

describe('pointsText', () => {
  it.each([
    [14, '+14'],
    [0, '0'],
    [-10, '−10'],
    [null, '–'],
  ])('writes %s as %s', (points, want) => {
    expect(pointsText(points)).toBe(want);
  });
});

describe('places', () => {
  it.each([
    ['one each', [30, 20, 10], [1, 2, 3]],
    ['a tie sharing a place, the next after them', [30, 20, 20, 10], [1, 2, 2, 4]],
    ['none', [], []],
  ])('gives %s', (_name, totals, want) => {
    expect(places(totals)).toEqual(want);
  });
});
