/** What a dialog asks, and its decisions: the answer its form gives, and which control has the focus as it opens. */

/** A question to confirm: its heading, the details, and the button that does it. */
export interface Question {
  heading: string;
  text?: string;
  /** The action's button, named for what it does ("Delete run"), never OK. */
  action: string;
  /** An action that cannot be undone: its button looks it, and Cancel has the focus. */
  final?: boolean;
}

/** A question wanting a typed answer, such as a ban's reason. */
export interface TextQuestion extends Question {
  field: string;
  required: boolean;
  maxLength?: number;
}

export type Focus = 'field' | 'cancel' | 'action';

/** hasField tells a question wanting a typed answer from one to confirm. */
export function hasField(q: Question | TextQuestion): q is TextQuestion {
  return 'field' in q;
}

/** answerOf is what the dialog's form answers: the field's text trimmed, '' for a question without one, or null when a required field is blank. */
export function answerOf(q: Question | TextQuestion, text: string): string | null {
  if (!hasField(q)) return '';
  const answer = text.trim();
  return q.required && answer === '' ? null : answer;
}

/** firstFocus is the control focused as the dialog opens: the field when there is one, Cancel before an action that cannot be undone, else the action. */
export function firstFocus(q: Question | TextQuestion): Focus {
  if (hasField(q)) return 'field';
  return q.final === true ? 'cancel' : 'action';
}
