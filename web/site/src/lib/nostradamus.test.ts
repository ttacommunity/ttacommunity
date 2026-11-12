import { describe, expect, it } from 'vitest';
import {
  chosenEdition,
  choose,
  draftFrom,
  answeredOf,
  editionLabel,
  editionParam,
  timeLeft,
  givenFrom,
  joinState,
  nostradamusHref,
  places,
  pointsText,
  pollShare,
  roundStage,
  shownName,
  stakeRange,
  staked,
  tokensShort,
  withStake,
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
    expect(draftFrom([pick, top2], [{ question: 2, answer: ['Ann', 'Cy'], tokens: 2 }])).toEqual({
      1: { answer: [], tokens: 0 },
      2: { answer: ['Ann', 'Cy'], tokens: 2 },
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
    expect(choose({ [q.id]: { answer: [...before], tokens: 0 } }, q, value)[q.id]?.answer).toEqual(want);
  });
});

describe('tokens', () => {
  const draft = { 1: { answer: ['Ann'], tokens: 2 }, 2: { answer: [], tokens: 0 }, 3: { answer: ['3'], tokens: 1 } };
  it('counts what is staked', () => {
    expect(staked(draft)).toBe(3);
  });
  it.each([
    ['up to what is left of the most', 2, 4, { min: 0, max: 1 }],
    ['what is already on it, plus what is left', 1, 4, { min: 0, max: 3 }],
    ['up to 99 with no most', 1, null, { min: 0, max: 99 }],
  ] as const)('lets an answer take %s', (_name, question, most, want) => {
    expect(stakeRange(draft, question, most)).toEqual(want);
  });
  it.each([
    ['sets a stake', 2, 1, 4, 1],
    ['keeps a stake within what is left', 2, 5, 4, 1],
    ['never goes below none', 1, -2, 4, 0],
  ] as const)('%s', (_name, question, tokens, most, want) => {
    expect(withStake(draft, question, tokens, most)[question]?.tokens).toBe(want);
  });
  it.each([
    ['nothing when the least is met', 3, 0],
    ['how many more the least asks for', 5, 2],
    ['nothing with no least', null, 0],
  ] as const)('is short by %s', (_name, least, want) => {
    expect(tokensShort(draft, least)).toBe(want);
  });
});

describe('givenFrom', () => {
  it('sends each answer complete, leaving out what is half done or empty', () => {
    const draft = { 1: { answer: ['Ann'], tokens: 2 }, 2: { answer: ['Cy'], tokens: 0 }, 3: { answer: [], tokens: 0 } };
    expect(givenFrom([pick, top2, number], draft)).toEqual([{ question: 1, answer: ['Ann'], tokens: 2 }]);
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

describe('timeLeft', () => {
  const now = new Date('2031-04-01T12:00:00Z');
  it.each([
    ['days and hours', '2031-04-03T15:00:00Z', '2 days 3 hours left'],
    ['a day', '2031-04-02T12:30:00Z', '1 day left'],
    ['hours and minutes', '2031-04-01T15:20:00Z', '3 hours 20 minutes left'],
    ['minutes', '2031-04-01T12:05:00Z', '5 minutes left'],
    ['none, once passed', '2031-04-01T11:00:00Z', 'closed'],
  ])('writes %s', (_name, closes, want) => {
    expect(timeLeft(closes, now)).toBe(want);
  });
});

describe('answeredOf', () => {
  it('counts the questions answered of those asked', () => {
    expect(answeredOf([{ id: 1 }, { id: 2 }, { id: 3 }], [{ question: 2 }, { question: 9 }])).toEqual({ answered: 1, of: 3 });
  });
});

describe('editionLabel', () => {
  const now = new Date('2031-04-01T12:00:00Z');
  it.each([
    ['a game with a round open', { name: 'Nostradamus #9', open_until: '2031-04-02T12:00:00Z' }, 'Nostradamus #9 · round open, 1 day left'],
    ['a game without', { name: 'Nostradamus #8', open_until: null }, 'Nostradamus #8'],
  ])('names %s', (_name, e, want) => {
    expect(editionLabel(e, now)).toBe(want);
  });
});
