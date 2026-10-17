import { describe, expect, it } from 'vitest';
import type { AdminResult } from './admin-api';
import { announcement, loadFailure, noticeHeading, outcomeOf, refusalOf, refused, revealed, shownAt, succeeded, type Outcome } from './outcome';

const OK: AdminResult<unknown> = { ok: true, data: null };
const NO: AdminResult<unknown> = { ok: false, status: 409, message: 'Refused: round 1 is not finished.' };

describe('outcomeOf', () => {
  it.each([
    ['done, saying what was done', OK, { kind: 'done', text: 'Ann Example is dropped.', at: 'entrant 7' }],
    ['refused, saying why', NO, { kind: 'refused', text: 'Refused: round 1 is not finished.', at: 'entrant 7' }],
  ])('is %s', (_, r, want) => {
    expect(outcomeOf(r, 'Ann Example is dropped.', 'entrant 7')).toEqual(want);
  });

  it.each([
    ['done shows where the removed row was listed', OK, 'entrants'],
    ['refused shows at the row still there', NO, 'entrant 7'],
  ])('is %s', (_, r, want) => {
    expect(outcomeOf(r, 'Removed Ann Example.', 'entrant 7', 'entrants').at).toBe(want);
  });
});

describe('refusalOf', () => {
  it.each([
    ['nothing for a request done', OK, null],
    ['the refusal of a request refused', NO, { kind: 'refused', text: 'Refused: round 1 is not finished.', at: 'note' }],
  ])('is %s', (_, r, want) => {
    expect(refusalOf(r, 'note')).toEqual(want);
  });
});

describe('outcomes made here', () => {
  it.each([
    ['an action done', succeeded('Round 1 updated.', 'rounds'), { kind: 'done', text: 'Round 1 updated.', at: 'rounds' }],
    ['a check refused before sending', refused('A ruling needs a reason.', 'match 3'), { kind: 'refused', text: 'A ruling needs a reason.', at: 'match 3' }],
    ['a load that failed', loadFailure('The admin site could not be reached.', 'page'), { kind: 'failed', text: 'The admin site could not be reached.', at: 'page' }],
  ])('keep their kind, text and place: %s', (_, got, want) => {
    expect(got).toEqual(want);
  });
});

describe('shownAt', () => {
  const o: Outcome = { kind: 'done', text: 'Saved.', at: 'entrant 7' };
  it.each([
    ['the outcome at its own place', o, 'entrant 7', o],
    ['nothing at another place', o, 'entrant 8', null],
    ['nothing when there is no outcome', null, 'entrant 7', null],
  ])('shows %s', (_, outcome, at, want) => {
    expect(shownAt(outcome, at)).toEqual(want);
  });
});

describe('noticeHeading', () => {
  it.each([
    ['done', 'Done'],
    ['refused', 'Not done'],
    ['failed', 'Could not load'],
  ] as const)('names a %s outcome in words', (kind, want) => {
    expect(noticeHeading(kind)).toBe(want);
  });
});

describe('revealed', () => {
  it.each([
    ['done', true],
    ['refused', true],
    ['failed', false],
  ] as const)('scrolls a %s outcome into view: %s', (kind, want) => {
    expect(revealed({ kind, text: '', at: '' })).toBe(want);
  });
});

describe('announcement', () => {
  it.each([
    ['a refusal at once', { kind: 'refused', text: 'Refused: round 1 is not finished.', at: '' }, { region: 'alert', text: 'Not done. Refused: round 1 is not finished.' }],
    ['a failed load at once', { kind: 'failed', text: 'The API failed (500).', at: '' }, { region: 'alert', text: 'Could not load. The API failed (500).' }],
    ['a success when the reader is free', { kind: 'done', text: 'Saved.', at: '' }, { region: 'status', text: 'Done. Saved.' }],
    ['nothing for no outcome', null, null],
  ] as const)('tells a screen reader %s', (_, o, want) => {
    expect(announcement(o)).toEqual(want);
  });
});
