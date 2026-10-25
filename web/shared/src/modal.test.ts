import { describe, expect, it } from 'vitest';
import { focusBack, type Opener } from './modal';

function opener(isConnected: boolean) {
  const o = { isConnected, focused: false, focus: () => (o.focused = true) };
  return o satisfies Opener;
}

describe('focusBack', () => {
  it.each([
    ['focuses an opener still on the page', true, true],
    ['leaves an opener gone from the page alone', false, false],
  ])('%s', (_name, connected, want) => {
    const o = opener(connected);
    focusBack(o);
    expect(o.focused).toBe(want);
  });

  it('does nothing without an opener', () => {
    expect(() => focusBack(null)).not.toThrow();
  });
});
