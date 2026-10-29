import { describe, expect, it } from 'vitest';
import { recallChoice, rememberChoice, shownLoad } from './challenge-view';

/** memory is a Storage holding what it is given. */
function memory(): Storage {
  const items = new Map<string, string>();
  return {
    get length() {
      return items.size;
    },
    clear: () => items.clear(),
    getItem: (k) => items.get(k) ?? null,
    key: (i) => [...items.keys()][i] ?? null,
    removeItem: (k) => void items.delete(k),
    setItem: (k, v) => void items.set(k, v),
  };
}

/** refusing is a Storage that throws, as private windows do. */
const refusing = {
  getItem: () => {
    throw new Error('blocked');
  },
  setItem: () => {
    throw new Error('blocked');
  },
} as unknown as Storage;

describe('recallChoice', () => {
  it.each([
    ['a remembered choice', 'measure', 'rating', 'rating'],
    ['the default for a value no longer offered', 'measure', 'average', 'total'],
    ['the default when nothing is remembered', 'measure', null, 'total'],
  ] as const)('gives %s', (_name, choice, saved, want) => {
    const store = memory();
    if (saved !== null) store.setItem(`ttac.challenges.${choice}`, saved);
    expect(recallChoice(store, choice)).toBe(want);
  });

  it('gives the default without a store, or from one that refuses', () => {
    expect(recallChoice(null, 'verified')).toBe('verified');
    expect(recallChoice(refusing, 'view')).toBe('cards');
  });
});

describe('rememberChoice', () => {
  it('keeps a choice for the next visit', () => {
    const store = memory();
    rememberChoice(store, 'verified', 'all');
    expect(recallChoice(store, 'verified')).toBe('all');
  });

  it('ignores a store that refuses', () => {
    expect(() => rememberChoice(refusing, 'view', 'compact')).not.toThrow();
  });
});

describe('shownLoad', () => {
  const ready = { kind: 'ready' as const, data: 1 };
  const loading = { kind: 'loading' as const };
  const failed = { kind: 'error' as const, message: 'down' };
  it.each([
    ['the last data while loading again', ready, loading, ready],
    ['loading the first time', null, loading, loading],
    ['new data once it is here', ready, { kind: 'ready' as const, data: 2 }, { kind: 'ready', data: 2 }],
    ['a failure, not old data', ready, failed, failed],
  ])('shows %s', (_name, last, next, want) => {
    expect(shownLoad(last, next)).toEqual(want);
  });
});
