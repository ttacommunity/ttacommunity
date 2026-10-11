import { describe, expect, it, vi } from 'vitest';
import { passBreaksOn } from './dev-proxy';

function emitter<Args extends unknown[]>() {
  const listeners: ((...args: Args) => void)[] = [];
  return {
    on: (_: string, listener: (...args: Args) => void) => listeners.push(listener),
    emit: (...args: Args) => listeners.forEach((listener) => listener(...args)),
  };
}

describe('passBreaksOn', () => {
  it.each([
    ['breaks off part way', false, 1],
    ['ends', true, 0],
  ])('when the API answer %s', (_, complete, want) => {
    const api = { complete, ...emitter<[]>() };
    const browser = { destroy: vi.fn() };
    const proxy = emitter<[typeof api, unknown, typeof browser]>();

    passBreaksOn(proxy);
    proxy.emit(api, {}, browser);
    api.emit();

    expect(browser.destroy).toHaveBeenCalledTimes(want);
  });
});
