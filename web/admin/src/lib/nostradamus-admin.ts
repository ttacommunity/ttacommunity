/** Admin's Nostradamus tab: an outcome as typed, read into the values the API takes, and the paths it changes. */

/** outcomeValues reads a typed outcome: a number whole, any other kind's names split on commas; none clears it. */
export function outcomeValues(kind: 'pick' | 'set' | 'number', text: string): string[] {
  if (kind === 'number') return text.trim() === '' ? [] : [text.trim()];
  return text
    .split(',')
    .map((v) => v.trim())
    .filter((v) => v !== '');
}

/** outcomeText writes an outcome for its field, empty while it is not known. */
export function outcomeText(outcome: readonly string[] | null | undefined): string {
  return outcome == null ? '' : outcome.join(', ');
}

/** outcomePath is where a question's outcome is set. */
export function outcomePath(edition: string, question: number): string {
  return `/api/nostradamus/${encodeURIComponent(edition)}/questions/${question}/outcome`;
}

/** memberPath is where a participant is linked to a member. */
export function memberPath(edition: string, participant: number): string {
  return `/api/nostradamus/${encodeURIComponent(edition)}/participants/${participant}/member`;
}
