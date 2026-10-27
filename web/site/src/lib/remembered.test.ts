import { describe, expect, it } from 'vitest';
import { DEFAULT_VIEW } from './leaderboard-view';
import { recall, remember } from './remembered';

function memory(): Storage {
  const m = new Map<string, string>();
  return {
    getItem: (k) => m.get(k) ?? null,
    setItem: (k, v) => void m.set(k, v),
    removeItem: (k) => void m.delete(k),
    clear: () => m.clear(),
    key: () => null,
    get length() {
      return m.size;
    },
  };
}

const broken = { getItem: () => { throw new Error('denied'); }, setItem: () => { throw new Error('denied'); } } as unknown as Storage;

describe('remember and recall', () => {
  it('keeps the engine, reset mode, and format, and nothing else', () => {
    const store = memory();
    remember(store, { ...DEFAULT_VIEW, engine: 'glicko2_std', reset: 'softer', format: 3, search: 'ann', page: 4 });
    expect(recall(store)).toEqual({ engine: 'glicko2_std', reset: 'softer', format: 3 });
  });

  it('recalls nothing from an empty or unreadable store, or a value it does not understand', () => {
    expect(recall(memory())).toEqual({});
    expect(recall(broken)).toEqual({});
    const store = memory();
    store.setItem('ttac.leaderboard', 'model=elo&format=9');
    expect(recall(store)).toEqual({});
  });

  it('does not throw when the store refuses to save', () => {
    expect(() => remember(broken, DEFAULT_VIEW)).not.toThrow();
  });

  it('works without a store, where the browser blocks it', () => {
    expect(recall(null)).toEqual({});
    expect(() => remember(null, DEFAULT_VIEW)).not.toThrow();
  });
});
