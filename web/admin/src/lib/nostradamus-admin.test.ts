import { describe, expect, it } from 'vitest';
import { choicesFromText, choicesText, closeBlocker, emptyQuestionForm, gameHref, outcomeText, outcomeValues, questionBody, questionFormFrom, roundStatus } from './nostradamus-admin';

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

describe('choices', () => {
  it('reads one per line, with a seed and a leaderboard rank when given', () => {
    expect(choicesFromText(' Ann | 2 | 30 \n\nCy\nDee | | 4\nEve | x')).toEqual([
      { label: 'Ann', seed: 2, leaderboard_rank: 30 },
      { label: 'Cy', seed: 0, leaderboard_rank: 0 },
      { label: 'Dee', seed: 0, leaderboard_rank: 4 },
      { label: 'Eve', seed: 0, leaderboard_rank: 0 },
    ]);
  });
  it('writes them back as they are read', () => {
    const choices = [
      { label: 'Ann', seed: 2, leaderboard_rank: 30 },
      { label: 'Cy', seed: 0, leaderboard_rank: 0 },
      { label: 'Dee', seed: 0, leaderboard_rank: 4 },
    ];
    expect(choicesFromText(choicesText(choices))).toEqual(choices);
  });
});

describe('the question form', () => {
  const q = { label: 'Top', prompt: 'Who are the top two?', kind: 'set' as const, points: 6, near_points: 0, picks: 2, asks_last: false, choices: [{ label: 'Ann', seed: 1, leaderboard_rank: 0 }] };
  it('fills from a question and sends it back the same', () => {
    expect(questionBody(questionFormFrom(q))).toEqual(q);
  });
  it.each([
    ['a pick takes one name', { ...emptyQuestionForm, kind: 'pick' as const, picks: 3 }, { picks: 1 }],
    ['a number has no choices or last place', { ...emptyQuestionForm, kind: 'number' as const, choices: 'Ann', asks_last: true }, { choices: [], asks_last: false }],
    ['only a number scores one off', { ...emptyQuestionForm, kind: 'pick' as const, near_points: 2 }, { near_points: 0 }],
  ])('sends %s', (_name, form, want) => {
    expect(questionBody(form)).toMatchObject(want);
  });
});

describe('roundStatus', () => {
  const now = new Date('2031-04-01T12:00:00Z');
  it.each([
    ['a draft', { published: false, closed: false, closes_at: null }, 'draft'],
    ['open before its deadline', { published: true, closed: false, closes_at: '2031-04-02T00:00:00Z' }, 'open'],
    ['awaiting its results after it', { published: true, closed: false, closes_at: '2031-04-01T11:00:00Z' }, 'awaiting'],
    ['closed', { published: true, closed: true, closes_at: '2031-04-01T11:00:00Z' }, 'closed'],
  ])('is %s', (_name, round, want) => {
    expect(roundStatus(round, now)).toBe(want);
  });
});

describe('closeBlocker', () => {
  const now = new Date('2031-04-01T12:00:00Z');
  const past = { published: true, closed: false, closes_at: '2031-04-01T11:00:00Z' };
  it.each([
    ['nothing once the deadline has passed and every outcome is in', { ...past, questions: [{ outcome: ['Ann'] }] }, ''],
    ['the deadline still to come', { ...past, closes_at: '2031-04-02T00:00:00Z', questions: [{ outcome: ['Ann'] }] }, 'Answers are open until the deadline.'],
    ['an outcome missing', { ...past, questions: [{ outcome: ['Ann'] }, { outcome: null }] }, 'Set every outcome first: 1 missing.'],
  ])('says %s', (_name, round, want) => {
    expect(closeBlocker(round, now)).toBe(want);
  });
});

describe('gameHref', () => {
  it("is the game's page in admin", () => {
    expect(gameHref('wc-2031')).toBe('/nostradamus/?edition=wc-2031');
  });
});
