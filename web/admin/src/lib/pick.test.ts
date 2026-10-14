import { describe, expect, it } from 'vitest';
import { enterAction } from './pick';

const open = { disabled: false };
const shut = { disabled: true };

describe('enterAction', () => {
  it.each([
    ['the option chosen with the arrow keys', [open, open], 1, false, { kind: 'pick', index: 1 }],
    ['nothing for a chosen option that may not be picked', [shut, open], 0, true, { kind: 'hold' }],
    ['the form sent when nothing is chosen or typed', [open, open], -1, false, { kind: 'send' }],
    ['the first that may be picked once something is typed', [shut, open], -1, true, { kind: 'pick', index: 1 }],
    ['the form held while nothing typed matches', [], -1, true, { kind: 'hold' }],
    ['the form held while nothing matched may be picked', [shut], -1, true, { kind: 'hold' }],
  ])('%s', (_, options, chosen, typed, want) => {
    expect(enterAction(options, chosen, typed)).toEqual(want);
  });
});
