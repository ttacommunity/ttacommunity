import { describe, expect, it } from 'vitest';
import { answerOf, firstFocus, type Question, type TextQuestion } from './question';

const confirm: Question = { heading: 'Remove A?', action: 'Remove' };
const final: Question = { heading: 'Delete run 1?', action: 'Delete run', final: true };
const required: TextQuestion = { heading: 'Ban A?', action: 'Ban', field: 'The reason', required: true };
const optional: TextQuestion = { heading: 'Refuse A?', action: 'Refuse', field: 'A reason', required: false };

describe('answerOf', () => {
  it.each([
    ['nothing for a question without a field', confirm, '', ''],
    ['the text, trimmed', required, '  spam  ', 'spam'],
    ['no answer when a required field is blank', required, '   ', null],
    ['an empty answer when an optional field is blank', optional, ' ', ''],
    ['the text of an optional field', optional, 'late', 'late'],
  ])('gives %s', (_name, q, text, want) => {
    expect(answerOf(q, text)).toBe(want);
  });
});

describe('firstFocus', () => {
  it.each([
    ['the action', confirm, 'action'],
    ['Cancel before an action that cannot be undone', final, 'cancel'],
    ['the field when an answer is wanted', required, 'field'],
    ['the field even before an action that cannot be undone', { ...required, final: true }, 'field'],
  ])('focuses %s', (_name, q, want) => {
    expect(firstFocus(q)).toBe(want);
  });
});
