import { describe, expect, it } from 'vitest';
import { choicesFromText, choicesText, closeBlocker, emptyQuestionForm, gameHref, outcomeText, outcomeValues, questionBody, questionFormFrom, countOrNone, gameSlug, linkChoices, tokensText, toggled, withChoice, roundLabel, roundStatus, seriesChoices } from './nostradamus-admin';

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

describe('roundLabel', () => {
  it.each([
    ['the one typed, trimmed', ' S8 ', ['S4', 'S7'], 'S8'],
    ['the first R free when none is typed', '', ['S4', 'R1', 'R2'], 'R3'],
    ['R1 in a new game', '  ', [], 'R1'],
  ])('is %s', (_name, typed, taken, want) => {
    expect(roundLabel(typed, taken)).toBe(want);
  });
});

describe('linkChoices', () => {
  const found = [
    { member: 4, name: 'Ann', player: 'Ann', discord_name: 'ann_d', email: '', country: null, zone: null, banned: false },
    { member: null, name: 'Rated_Rae', player: 'Rated_Rae', discord_name: '', email: '', country: null, zone: null, banned: false },
  ];
  it('offers the members and rated players found, not a new name, which a link cannot make', () => {
    expect(linkChoices(found, 'Zed').map((c) => c.label)).toEqual(['Ann', 'Rated_Rae']);
  });
});

describe('seriesChoices', () => {
  const list = [
    { slug: 'worlds', name: 'World Championship', is_group: false },
    { slug: 'us_open', name: 'US Open', is_group: false },
    { slug: 'opens', name: 'Opens', is_group: true },
  ];
  it.each([
    ['a series whose name holds the text, in any case', 'open', ['US Open']],
    ['a series by its slug', 'worl', ['World Championship']],
    ['every series, not a group, for no text', '', ['World Championship', 'US Open']],
  ])('offers %s', (_name, typed, want) => {
    expect(seriesChoices(list, typed).map((c) => c.label)).toEqual(want);
  });
});

describe('gameSlug', () => {
  it.each([
    ['a name, lower case with dashes', 'Nostradamus #6: WC 2027!', 'nostradamus-6-wc-2027'],
    ['no name', '  ', ''],
  ])('makes %s', (_name, name, want) => {
    expect(gameSlug(name)).toBe(want);
  });
});

describe('withChoice', () => {
  it.each([
    ['adds a name on its own line', 'Ann | 2', 'Cy', 'Ann | 2\nCy'],
    ['starts the list', '', 'Cy', 'Cy'],
    ['leaves a name already there, in any case', 'Ann\ncy | 3', 'Cy', 'Ann\ncy | 3'],
  ])('%s', (_name, text, name, want) => {
    expect(withChoice(text, name)).toBe(want);
  });
});

describe('toggled', () => {
  it.each([
    ['ticks a choice', ['Ann'], 'Cy', 0, ['Ann', 'Cy']],
    ['unticks one ticked', ['Ann', 'Cy'], 'Ann', 0, ['Cy']],
    ['takes no more past the limit', ['Ann', 'Cy'], 'Dee', 2, ['Ann', 'Cy']],
    ['swaps a single pick', ['Ann'], 'Cy', 1, ['Cy']],
  ])('%s', (_name, values, value, limit, want) => {
    expect(toggled(values, value, limit)).toEqual(want);
  });
});

describe('countOrNone', () => {
  it.each([
    ['a whole number', ' 3 ', 3],
    ['nothing, for no bound', '  ', null],
    ['nothing for what is not a whole number', '2.5', null],
  ])('reads %s', (_name, text, want) => {
    expect(countOrNone(text)).toBe(want);
  });
});

describe('tokensText', () => {
  it.each([
    ['both bounds', 1, 3, 10, '1 to 3 tokens, ±10 each'],
    ['a most only', null, 3, 10, 'up to 3 tokens, ±10 each'],
    ['a least only', 2, null, 5, 'at least 2 tokens, ±5 each'],
    ['no bounds', null, null, 10, 'any number of tokens, ±10 each'],
    ['none allowed', null, 0, 10, 'no tokens'],
  ])('says %s', (_name, min, max, points, want) => {
    expect(tokensText(min, max, points)).toBe(want);
  });
});
