import { describe, expect, it } from 'vitest';
import {
  chosenEdition,
  choose,
  draftFrom,
  editionParam,
  givenFrom,
  joinState,
  nostradamusHref,
  places,
  pointsText,
  pollShare,
  roundStage,
  shownName,
  tokensLeft,
  toggleToken,
  valuesText,
} from './nostradamus';

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
    ['?edition=wc-2031', 'wc-2031'],
    ['?edition=%20', null],
    ['', null],
  ])('reads %s', (search, want) => {
    expect(editionParam(search)).toBe(want);
  });
});

describe('nostradamusHref', () => {
  it('names the edition', () => {
    expect(nostradamusHref('wc-2031')).toBe('/nostradamus/?edition=wc-2031');
  });
});

describe('shownName', () => {
  it.each([
    ['the member it is linked to', { name: 'ann', member: 'Ann' }, 'Ann'],
    ['the name it plays under, unlinked', { name: 'Dee', member: null }, 'Dee'],
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

describe('roundStage', () => {
  const now = new Date('2031-04-01T12:00:00Z');
  it.each([
    ['open before its deadline', { published: true, closed: false, closes_at: '2031-04-02T00:00:00Z' }, 'open'],
    ['awaiting results after it', { published: true, closed: false, closes_at: '2031-04-01T11:59:00Z' }, 'awaiting'],
    ['closed once its results are out', { published: true, closed: true, closes_at: '2031-03-01T00:00:00Z' }, 'closed'],
  ])('is %s', (_name, round, want) => {
    expect(roundStage(round, now)).toBe(want);
  });
});

describe('joinState', () => {
  it.each([
    ['signed out', false, false, 'none', 'sign-in'],
    ['signed in, not linked to a TTA username', true, false, 'none', 'link'],
    ['linked, not asked yet', true, true, 'none', 'ask'],
    ['asked', true, true, 'requested', 'requested'],
    ['refused', true, true, 'refused', 'refused'],
    ['accepted', true, true, 'accepted', 'play'],
  ] as const)('is %s', (_name, signedIn, linked, status, want) => {
    expect(joinState(signedIn, linked, status)).toBe(want);
  });
});

const pick = { id: 1, kind: 'pick', picks: 1 } as const;
const top2 = { id: 2, kind: 'set', picks: 2 } as const;
const number = { id: 3, kind: 'number', picks: 1 } as const;

describe('draftFrom', () => {
  it("starts from the player's saved answers, empty elsewhere", () => {
    expect(draftFrom([pick, top2], [{ question: 2, answer: ['Ann', 'Cy'], token: true }])).toEqual({
      1: { answer: [], token: false },
      2: { answer: ['Ann', 'Cy'], token: true },
    });
  });
});

describe('choose', () => {
  it.each([
    ['a pick replaces the last', pick, ['Ann'], 'Cy', ['Cy']],
    ['a set adds a name', top2, ['Ann'], 'Cy', ['Ann', 'Cy']],
    ['a set drops a name chosen again', top2, ['Ann', 'Cy'], 'Ann', ['Cy']],
    ['a full set takes no more', top2, ['Ann', 'Cy'], 'Dee', ['Ann', 'Cy']],
    ['a number takes what is typed', number, ['3'], '12', ['12']],
    ['a number cleared is none', number, ['3'], ' ', []],
  ] as const)('%s', (_name, q, before, value, want) => {
    expect(choose({ [q.id]: { answer: [...before], token: false } }, q, value)[q.id]?.answer).toEqual(want);
  });
});

describe('tokens', () => {
  const draft = { 1: { answer: ['Ann'], token: true }, 2: { answer: [], token: false }, 3: { answer: ['3'], token: false } };
  it('counts what is left of the round', () => {
    expect(tokensLeft(draft, 2)).toBe(1);
  });
  it.each([
    ['puts one on while some are left', 2, 3, true],
    ['takes one off', 1, 1, false],
    ['puts none on when none is left', 1, 3, false],
  ] as const)('%s', (_name, tokens, question, want) => {
    expect(toggleToken(draft, question, tokens)[question]?.token).toBe(want);
  });
});

describe('givenFrom', () => {
  it('sends each answer complete, leaving out what is half done or empty', () => {
    const draft = { 1: { answer: ['Ann'], token: true }, 2: { answer: ['Cy'], token: false }, 3: { answer: [], token: false } };
    expect(givenFrom([pick, top2, number], draft)).toEqual([{ question: 1, answer: ['Ann'], token: true }]);
  });
});

describe('pollShare', () => {
  const q = { counts: [{ value: 'Ann', answers: 3 }, { value: 'Cy', answers: 1 }] };
  it.each([
    ['a name picked', 'ann', { answers: 3, share: 75 }],
    ['a name nobody picked', 'Dee', { answers: 0, share: 0 }],
  ])('gives %s', (_name, label, want) => {
    expect(pollShare(q, label)).toEqual(want);
  });
});
