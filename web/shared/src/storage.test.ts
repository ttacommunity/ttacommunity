import { afterEach, describe, expect, it, vi } from 'vitest';
import { browserStorage } from './storage';

describe('browserStorage', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('is the local storage where the browser allows it', () => {
    const local = { getItem: () => null } as unknown as Storage;
    vi.stubGlobal('window', { localStorage: local });
    expect(browserStorage()).toBe(local);
  });

  it.each([
    ['touching it throws (blocked site data)', { get localStorage(): Storage { throw new DOMException('denied', 'SecurityError'); } }],
    ['there is no window', undefined],
  ])('is null where %s', (_name, win) => {
    vi.stubGlobal('window', win);
    expect(browserStorage()).toBeNull();
  });
});
